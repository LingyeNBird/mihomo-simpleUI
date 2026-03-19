package model

import "time"

type Subscription struct {
	ID              int64      `json:"id"`
	Name            string     `json:"name"`
	URL             string     `json:"url"`
	Enabled         bool       `json:"enabled"`
	FilePath        string     `json:"filePath"`
	LastRefreshedAt *time.Time `json:"lastRefreshedAt,omitempty"`
	LastError       string     `json:"lastError,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	UpdatedAt       time.Time  `json:"updatedAt"`
}

type Delay struct {
	Time  string `json:"time"`
	Delay int    `json:"delay"`
}

type ProxyGroup struct {
	Name        string              `json:"name"`
	Type        string              `json:"type"`
	Current     string              `json:"current"`
	All         []string            `json:"all"`
	NodeSources map[string][]string `json:"nodeSources,omitempty"`
	History     []Delay             `json:"history,omitempty"`
}

type Selection struct {
	GroupName  string    `json:"groupName"`
	NodeName   string    `json:"nodeName"`
	ModifiedAt time.Time `json:"modifiedAt"`
}

type AppStatus struct {
	AppStartedAt        time.Time `json:"appStartedAt"`
	ConfigPath          string    `json:"configPath"`
	DatabasePath        string    `json:"databasePath"`
	MihomoControllerURL string    `json:"mihomoControllerUrl"`
	MihomoConnected     bool      `json:"mihomoConnected"`
	MihomoVersion       string    `json:"mihomoVersion,omitempty"`
	SubscriptionCount   int       `json:"subscriptionCount"`
	LastConfigSyncAt    time.Time `json:"lastConfigSyncAt"`
	LastConfigError     string    `json:"lastConfigError,omitempty"`
}

type ConfigSyncResult struct {
	ConfigPath string   `json:"configPath"`
	Reloaded   bool     `json:"reloaded"`
	Warnings   []string `json:"warnings,omitempty"`
}

type SubscriptionContent struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	FilePath string `json:"filePath"`
	Content  string `json:"content"`
}

type AuthUser struct {
	Username           string    `json:"username"`
	PasswordHash       string    `json:"-"`
	MustChangePassword bool      `json:"mustChangePassword"`
	PasswordChangedAt  time.Time `json:"passwordChangedAt"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

type Session struct {
	ID        string    `json:"id"`
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type AuthStatus struct {
	Authenticated      bool   `json:"authenticated"`
	MustChangePassword bool   `json:"mustChangePassword"`
	Username           string `json:"username,omitempty"`
}
