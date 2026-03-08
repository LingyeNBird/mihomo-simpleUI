package service

import (
	"context"
	"os"
	"time"

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
	downloader      *subscription.Downloader
	mihomoClient    *mihomo.Client
	lastConfigSync  time.Time
	lastConfigError string
}

func New(cfg config.Config, st *store.SQLiteStore) *Service {
	return &Service{
		config:       cfg,
		store:        st,
		downloader:   subscription.NewDownloader(cfg.RequestTimeout, cfg.SubscriptionsDir),
		mihomoClient: mihomo.NewClient(cfg.MihomoControllerURL, cfg.MihomoSecret, cfg.RequestTimeout),
	}
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
	return s.mihomoClient.GetSelectableGroups(ctx)
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
