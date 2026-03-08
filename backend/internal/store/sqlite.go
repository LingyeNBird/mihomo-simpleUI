package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"mihomo-webui-proxy/backend/internal/model"
)

type SQLiteStore struct {
	db *sql.DB
}

type scanner interface {
	Scan(dest ...any) error
}

func NewSQLiteStore(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(time.Minute)

	store := &SQLiteStore{db: db}
	if err := store.migrate(context.Background()); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func (s *SQLiteStore) migrate(ctx context.Context) error {
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			url TEXT NOT NULL UNIQUE,
			enabled INTEGER NOT NULL DEFAULT 1,
			file_path TEXT NOT NULL,
			last_refreshed_at TEXT,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS selections (
			group_name TEXT PRIMARY KEY,
			node_name TEXT NOT NULL,
			modified_at TEXT NOT NULL
		);`,
	} {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate sqlite: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) ListSubscriptions(ctx context.Context) ([]model.Subscription, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, url, enabled, file_path, last_refreshed_at, last_error, created_at, updated_at FROM subscriptions ORDER BY id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query subscriptions: %w", err)
	}
	defer rows.Close()

	var items []model.Subscription
	for rows.Next() {
		item, err := scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *SQLiteStore) GetSubscription(ctx context.Context, id int64) (model.Subscription, error) {
	return scanSubscription(s.db.QueryRowContext(ctx, `SELECT id, name, url, enabled, file_path, last_refreshed_at, last_error, created_at, updated_at FROM subscriptions WHERE id = ?`, id))
}

func (s *SQLiteStore) CreateSubscription(ctx context.Context, item model.Subscription) (model.Subscription, error) {
	now := time.Now().UTC()
	item.CreatedAt = now
	item.UpdatedAt = now
	result, err := s.db.ExecContext(ctx, `INSERT INTO subscriptions (name, url, enabled, file_path, last_refreshed_at, last_error, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, item.Name, item.URL, boolToInt(item.Enabled), item.FilePath, nullableTime(item.LastRefreshedAt), item.LastError, item.CreatedAt.Format(time.RFC3339), item.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return model.Subscription{}, fmt.Errorf("insert subscription: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return model.Subscription{}, fmt.Errorf("subscription last insert id: %w", err)
	}
	item.ID = id
	return item, nil
}

func (s *SQLiteStore) UpdateSubscription(ctx context.Context, item model.Subscription) (model.Subscription, error) {
	item.UpdatedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `UPDATE subscriptions SET name = ?, url = ?, enabled = ?, file_path = ?, last_refreshed_at = ?, last_error = ?, updated_at = ? WHERE id = ?`, item.Name, item.URL, boolToInt(item.Enabled), item.FilePath, nullableTime(item.LastRefreshedAt), item.LastError, item.UpdatedAt.Format(time.RFC3339), item.ID)
	if err != nil {
		return model.Subscription{}, fmt.Errorf("update subscription: %w", err)
	}
	return item, nil
}

func (s *SQLiteStore) DeleteSubscription(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete subscription: %w", err)
	}
	return nil
}

func (s *SQLiteStore) SaveSelection(ctx context.Context, selection model.Selection) error {
	selection.ModifiedAt = time.Now().UTC()
	_, err := s.db.ExecContext(ctx, `INSERT INTO selections (group_name, node_name, modified_at) VALUES (?, ?, ?) ON CONFLICT(group_name) DO UPDATE SET node_name = excluded.node_name, modified_at = excluded.modified_at`, selection.GroupName, selection.NodeName, selection.ModifiedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("save selection: %w", err)
	}
	return nil
}

func (s *SQLiteStore) ListSelections(ctx context.Context) ([]model.Selection, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT group_name, node_name, modified_at FROM selections ORDER BY group_name ASC`)
	if err != nil {
		return nil, fmt.Errorf("query selections: %w", err)
	}
	defer rows.Close()

	var items []model.Selection
	for rows.Next() {
		var item model.Selection
		var modifiedAt string
		if err := rows.Scan(&item.GroupName, &item.NodeName, &modifiedAt); err != nil {
			return nil, fmt.Errorf("scan selection: %w", err)
		}
		parsed, err := time.Parse(time.RFC3339, modifiedAt)
		if err != nil {
			return nil, fmt.Errorf("parse selection time: %w", err)
		}
		item.ModifiedAt = parsed
		items = append(items, item)
	}
	return items, rows.Err()
}

func scanSubscription(s scanner) (model.Subscription, error) {
	var item model.Subscription
	var enabled int
	var createdAt string
	var updatedAt string
	var lastRefreshed sql.NullString
	err := s.Scan(&item.ID, &item.Name, &item.URL, &enabled, &item.FilePath, &lastRefreshed, &item.LastError, &createdAt, &updatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return model.Subscription{}, fmt.Errorf("subscription not found")
		}
		return model.Subscription{}, fmt.Errorf("scan subscription: %w", err)
	}
	item.Enabled = enabled == 1
	created, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return model.Subscription{}, fmt.Errorf("parse created at: %w", err)
	}
	updated, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return model.Subscription{}, fmt.Errorf("parse updated at: %w", err)
	}
	item.CreatedAt = created
	item.UpdatedAt = updated
	if lastRefreshed.Valid {
		parsed, err := time.Parse(time.RFC3339, lastRefreshed.String)
		if err != nil {
			return model.Subscription{}, fmt.Errorf("parse last refreshed at: %w", err)
		}
		item.LastRefreshedAt = &parsed
	}
	return item, nil
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
