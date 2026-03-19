package service

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"mihomo-webui-proxy/backend/internal/auth"
	"mihomo-webui-proxy/backend/internal/config"
	"mihomo-webui-proxy/backend/internal/generator"
	"mihomo-webui-proxy/backend/internal/mihomo"
	"mihomo-webui-proxy/backend/internal/model"
	"mihomo-webui-proxy/backend/internal/store"
	"mihomo-webui-proxy/backend/internal/subscription"
)

type Service struct {
	config          config.Config
	store           *store.SQLiteStore
	authManager     *auth.Manager
	downloader      *subscription.Downloader
	mihomoClient    *mihomo.Client
	lastConfigSync  time.Time
	lastConfigError string
}

func New(cfg config.Config, st *store.SQLiteStore) (*Service, error) {
	authManager := auth.NewManager(st)
	if err := authManager.EnsureInitialUser(context.Background()); err != nil {
		return nil, fmt.Errorf("ensure initial auth user: %w", err)
	}
	return &Service{
		config:       cfg,
		store:        st,
		authManager:  authManager,
		downloader:   subscription.NewDownloader(cfg.RequestTimeout, cfg.SubscriptionsDir),
		mihomoClient: mihomo.NewClient(cfg.MihomoControllerURL, cfg.MihomoSecret, cfg.RequestTimeout),
	}, nil
}

func (s *Service) Login(ctx context.Context, username, password string) (model.AuthStatus, model.Session, error) {
	user, session, err := s.authManager.Authenticate(ctx, username, password)
	if err != nil {
		return model.AuthStatus{}, model.Session{}, err
	}
	return model.AuthStatus{Authenticated: true, MustChangePassword: user.MustChangePassword, Username: user.Username}, session, nil
}

func (s *Service) Logout(ctx context.Context, sessionID string) error {
	return s.authManager.Logout(ctx, sessionID)
}

func (s *Service) AuthStatus(ctx context.Context, sessionID string) model.AuthStatus {
	user, _, err := s.authManager.GetSessionUser(ctx, sessionID)
	if err != nil {
		return model.AuthStatus{}
	}
	return model.AuthStatus{Authenticated: true, MustChangePassword: user.MustChangePassword, Username: user.Username}
}

func (s *Service) ChangePassword(ctx context.Context, sessionID, currentPassword, newPassword string) (model.AuthStatus, error) {
	user, _, err := s.authManager.GetSessionUser(ctx, sessionID)
	if err != nil {
		return model.AuthStatus{}, err
	}
	updated, err := s.authManager.ChangePassword(ctx, user.Username, currentPassword, newPassword)
	if err != nil {
		return model.AuthStatus{}, err
	}
	return model.AuthStatus{Authenticated: true, MustChangePassword: updated.MustChangePassword, Username: updated.Username}, nil
}

func (s *Service) ListSubscriptions(ctx context.Context) ([]model.Subscription, error) {
	return s.store.ListSubscriptions(ctx)
}

func (s *Service) CreateSubscription(ctx context.Context, name, rawURL string, enabled bool) (subscription.RefreshResult, error) {
	item := model.Subscription{Name: name, URL: rawURL, Enabled: enabled, FilePath: "/config/subscriptions/pending.yaml"}
	created, err := s.store.CreateSubscription(ctx, item)
	if err != nil {
		return subscription.RefreshResult{}, err
	}
	created.FilePath = subscription.FilePathFor(s.config.SubscriptionsDir, created.Name, created.ID)
	created, err = s.store.UpdateSubscription(ctx, created)
	if err != nil {
		return subscription.RefreshResult{}, err
	}
	refresh, err := s.downloader.Download(ctx, created)
	if err != nil {
		created.LastError = err.Error()
		_, _ = s.store.UpdateSubscription(ctx, created)
		return subscription.RefreshResult{}, err
	}
	updated, err := s.store.UpdateSubscription(ctx, refresh.Subscription)
	if err != nil {
		return subscription.RefreshResult{}, err
	}
	refresh.Subscription = updated
	if _, err := s.SyncConfig(ctx); err != nil {
		return subscription.RefreshResult{}, err
	}
	return refresh, nil
}

func (s *Service) UpdateSubscription(ctx context.Context, id int64, name, rawURL string, enabled bool) (model.Subscription, error) {
	item, err := s.store.GetSubscription(ctx, id)
	if err != nil {
		return model.Subscription{}, err
	}
	item.Name = name
	item.URL = rawURL
	item.Enabled = enabled
	item.FilePath = subscription.FilePathFor(s.config.SubscriptionsDir, item.Name, item.ID)
	updated, err := s.store.UpdateSubscription(ctx, item)
	if err != nil {
		return model.Subscription{}, err
	}
	if updated.Enabled {
		if _, err := s.RefreshSubscription(ctx, id); err != nil {
			return model.Subscription{}, err
		}
	} else if _, err := s.SyncConfig(ctx); err != nil {
		return model.Subscription{}, err
	}
	return s.store.GetSubscription(ctx, id)
}

func (s *Service) DeleteSubscription(ctx context.Context, id int64) error {
	item, err := s.store.GetSubscription(ctx, id)
	if err != nil {
		return err
	}
	if item.FilePath != "" {
		_ = os.Remove(item.FilePath)
	}
	if err := s.store.DeleteSubscription(ctx, id); err != nil {
		return err
	}
	_, err = s.SyncConfig(ctx)
	return err
}

func (s *Service) RefreshSubscription(ctx context.Context, id int64) (subscription.RefreshResult, error) {
	item, err := s.store.GetSubscription(ctx, id)
	if err != nil {
		return subscription.RefreshResult{}, err
	}
	refresh, err := s.downloader.Download(ctx, item)
	if err != nil {
		item.LastError = err.Error()
		_, _ = s.store.UpdateSubscription(ctx, item)
		return subscription.RefreshResult{}, err
	}
	updated, err := s.store.UpdateSubscription(ctx, refresh.Subscription)
	if err != nil {
		return subscription.RefreshResult{}, err
	}
	refresh.Subscription = updated
	if _, err := s.SyncConfig(ctx); err != nil {
		return subscription.RefreshResult{}, err
	}
	return refresh, nil
}

func (s *Service) GetSubscriptionContent(ctx context.Context, id int64) (model.SubscriptionContent, error) {
	item, err := s.store.GetSubscription(ctx, id)
	if err != nil {
		return model.SubscriptionContent{}, err
	}
	content, err := os.ReadFile(item.FilePath)
	if err != nil {
		return model.SubscriptionContent{}, fmt.Errorf("read subscription content: %w", err)
	}
	return model.SubscriptionContent{
		ID:       item.ID,
		Name:     item.Name,
		FilePath: item.FilePath,
		Content:  string(content),
	}, nil
}

func (s *Service) SyncConfig(ctx context.Context) (model.ConfigSyncResult, error) {
	subscriptions, err := s.store.ListSubscriptions(ctx)
	if err != nil {
		return model.ConfigSyncResult{}, err
	}
	selections, err := s.store.ListSelections(ctx)
	if err != nil {
		return model.ConfigSyncResult{}, err
	}
	generated, err := generator.Generate(subscriptions, selections, generator.Options{
		MixedPort:      s.config.MihomoMixedPort,
		ControllerPort: s.config.MihomoExternalPort,
		Secret:         s.config.MihomoSecret,
		Mode:           s.config.MihomoProxyMode,
		ConfigPath:     s.config.GeneratedConfigPath,
	})
	if err != nil {
		s.lastConfigError = err.Error()
		return model.ConfigSyncResult{}, err
	}
	reloaded := false
	if err := s.mihomoClient.ReloadConfig(ctx, s.config.GeneratedConfigPath); err == nil {
		reloaded = true
	} else {
		s.lastConfigError = err.Error()
	}
	s.lastConfigSync = time.Now().UTC()
	return model.ConfigSyncResult{ConfigPath: s.config.GeneratedConfigPath, Reloaded: reloaded, Warnings: generated.Warnings}, nil
}

func (s *Service) GetProxyGroups(ctx context.Context) ([]model.ProxyGroup, error) {
	groups, err := s.mihomoClient.GetSelectableGroups(ctx)
	if err != nil {
		return nil, err
	}

	subscriptions, err := s.store.ListSubscriptions(ctx)
	if err != nil {
		return nil, err
	}

	nodeSources := make(map[string][]string)
	for _, sub := range subscriptions {
		if !sub.Enabled || sub.FilePath == "" {
			continue
		}
		proxyNames, _, err := generator.LoadSubscriptionProxies(sub.FilePath)
		if err != nil {
			continue
		}
		for _, name := range proxyNames {
			nodeSources[name] = appendUniqueString(nodeSources[name], sub.Name)
		}
	}

	for index := range groups {
		groups[index].NodeSources = make(map[string][]string)
		for _, nodeName := range groups[index].All {
			if sources := nodeSources[nodeName]; len(sources) > 0 {
				groups[index].NodeSources[nodeName] = append([]string(nil), sources...)
			}
		}
	}

	return groups, nil
}

func (s *Service) SelectNode(ctx context.Context, groupName, nodeName string) error {
	if err := s.mihomoClient.SelectNode(ctx, groupName, nodeName); err != nil {
		return err
	}
	if err := s.store.SaveSelection(ctx, model.Selection{GroupName: groupName, NodeName: nodeName}); err != nil {
		return err
	}
	_, err := s.SyncConfig(ctx)
	return err
}

func (s *Service) GetStatus(ctx context.Context) model.AppStatus {
	subscriptions, _ := s.store.ListSubscriptions(ctx)
	version, err := s.mihomoClient.GetVersion(ctx)
	return model.AppStatus{
		AppStartedAt:        s.config.AppStartedAt,
		ConfigPath:          s.config.GeneratedConfigPath,
		DatabasePath:        s.config.DBPath,
		MihomoControllerURL: s.config.MihomoControllerURL,
		MihomoConnected:     err == nil,
		MihomoVersion:       version,
		SubscriptionCount:   len(subscriptions),
		LastConfigSyncAt:    s.lastConfigSync,
		LastConfigError:     s.lastConfigError,
	}
}

func appendUniqueString(items []string, value string) []string {
	for _, item := range items {
		if item == value {
			return items
		}
	}
	items = append(items, value)
	sort.Strings(items)
	return items
}
