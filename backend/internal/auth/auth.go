package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"mihomo-webui-proxy/backend/internal/model"
	"mihomo-webui-proxy/backend/internal/store"
)

const (
	InitialUsername = "mihomo"
	InitialPassword = "mihomo"
	SessionTTL      = 7 * 24 * time.Hour
)

type Manager struct {
	store *store.SQLiteStore
}

func NewManager(st *store.SQLiteStore) *Manager {
	return &Manager{store: st}
}

func (m *Manager) EnsureInitialUser(ctx context.Context) error {
	if _, err := m.store.GetAuthUser(ctx, InitialUsername); err == nil {
		return nil
	}
	hash, err := HashPassword(InitialPassword)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err = m.store.UpsertAuthUser(ctx, model.AuthUser{
		Username:           InitialUsername,
		PasswordHash:       hash,
		MustChangePassword: true,
		PasswordChangedAt:  now,
		CreatedAt:          now,
		UpdatedAt:          now,
	})
	if err != nil {
		return fmt.Errorf("seed initial auth user: %w", err)
	}
	return nil
}

func (m *Manager) Authenticate(ctx context.Context, username, password string) (model.AuthUser, model.Session, error) {
	if err := m.store.DeleteExpiredSessions(ctx, time.Now().UTC()); err != nil {
		return model.AuthUser{}, model.Session{}, err
	}
	user, err := m.store.GetAuthUser(ctx, strings.TrimSpace(username))
	if err != nil {
		return model.AuthUser{}, model.Session{}, fmt.Errorf("invalid username or password")
	}
	if err := ComparePassword(user.PasswordHash, password); err != nil {
		return model.AuthUser{}, model.Session{}, fmt.Errorf("invalid username or password")
	}
	session, err := newSession(user.Username)
	if err != nil {
		return model.AuthUser{}, model.Session{}, err
	}
	if err := m.store.CreateSession(ctx, session); err != nil {
		return model.AuthUser{}, model.Session{}, err
	}
	return user, session, nil
}

func (m *Manager) GetSessionUser(ctx context.Context, sessionID string) (model.AuthUser, model.Session, error) {
	if sessionID == "" {
		return model.AuthUser{}, model.Session{}, fmt.Errorf("missing session")
	}
	session, err := m.store.GetSession(ctx, sessionID)
	if err != nil {
		return model.AuthUser{}, model.Session{}, err
	}
	now := time.Now().UTC()
	if !session.ExpiresAt.After(now) {
		_ = m.store.DeleteSession(ctx, session.ID)
		return model.AuthUser{}, model.Session{}, fmt.Errorf("session expired")
	}
	user, err := m.store.GetAuthUser(ctx, session.Username)
	if err != nil {
		return model.AuthUser{}, model.Session{}, err
	}
	return user, session, nil
}

func (m *Manager) ChangePassword(ctx context.Context, username, currentPassword, newPassword string) (model.AuthUser, error) {
	user, err := m.store.GetAuthUser(ctx, username)
	if err != nil {
		return model.AuthUser{}, err
	}
	if err := ComparePassword(user.PasswordHash, currentPassword); err != nil {
		return model.AuthUser{}, fmt.Errorf("current password is incorrect")
	}
	newPassword = strings.TrimSpace(newPassword)
	if len(newPassword) < 6 {
		return model.AuthUser{}, fmt.Errorf("new password must be at least 6 characters")
	}
	hash, err := HashPassword(newPassword)
	if err != nil {
		return model.AuthUser{}, err
	}
	user.PasswordHash = hash
	user.MustChangePassword = false
	user.PasswordChangedAt = time.Now().UTC()
	return m.store.UpsertAuthUser(ctx, user)
}

func (m *Manager) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	return m.store.DeleteSession(ctx, sessionID)
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password: %w", err)
	}
	return string(hash), nil
}

func ComparePassword(hash string, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
}

func newSession(username string) (model.Session, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return model.Session{}, fmt.Errorf("generate session id: %w", err)
	}
	now := time.Now().UTC()
	return model.Session{
		ID:        base64.RawURLEncoding.EncodeToString(raw),
		Username:  username,
		CreatedAt: now,
		ExpiresAt: now.Add(SessionTTL),
	}, nil
}
