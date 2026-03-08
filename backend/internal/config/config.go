package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

type Config struct {
	ListenAddr          string
	DataDir             string
	DBPath              string
	ConfigDir           string
	SubscriptionsDir    string
	GeneratedConfigPath string
	StaticDir           string
	MihomoControllerURL string
	MihomoSecret        string
	MihomoMixedPort     int
	MihomoProxyMode     string
	MihomoExternalPort  int
	RequestTimeout      time.Duration
	HealthcheckURL      string
	AppStartedAt        time.Time
}

func Load() Config {
	dataDir := envOrDefault("APP_DATA_DIR", "/app/data")
	configDir := envOrDefault("APP_CONFIG_DIR", filepath.Join(dataDir, "config"))
	requestTimeoutSeconds := envIntOrDefault("APP_REQUEST_TIMEOUT_SECONDS", 20)

	return Config{
		ListenAddr:          envOrDefault("APP_LISTEN_ADDR", ":8080"),
		DataDir:             dataDir,
		DBPath:              envOrDefault("APP_DB_PATH", filepath.Join(dataDir, "db", "app.db")),
		ConfigDir:           configDir,
		SubscriptionsDir:    envOrDefault("APP_SUBSCRIPTIONS_DIR", filepath.Join(configDir, "subscriptions")),
		GeneratedConfigPath: envOrDefault("APP_GENERATED_CONFIG_PATH", filepath.Join(configDir, "config.yaml")),
		StaticDir:           envOrDefault("APP_STATIC_DIR", "/app/static"),
		MihomoControllerURL: envOrDefault("MIHOMO_CONTROLLER_URL", "http://mihomo:9090"),
		MihomoSecret:        envOrDefault("MIHOMO_SECRET", "change-me"),
		MihomoMixedPort:     envIntOrDefault("MIHOMO_MIXED_PORT", 7890),
		MihomoProxyMode:     envOrDefault("MIHOMO_PROXY_MODE", "rule"),
		MihomoExternalPort:  envIntOrDefault("MIHOMO_EXTERNAL_PORT", 9090),
		RequestTimeout:      time.Duration(requestTimeoutSeconds) * time.Second,
		HealthcheckURL:      envOrDefault("MIHOMO_HEALTHCHECK_URL", "https://www.gstatic.com/generate_204"),
		AppStartedAt:        time.Now().UTC(),
	}
}

func (c Config) EnsurePaths() error {
	for _, dir := range []string{c.DataDir, filepath.Dir(c.DBPath), c.ConfigDir, c.SubscriptionsDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create dir %s: %w", dir, err)
		}
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envIntOrDefault(key string, fallback int) int {
	if value := os.Getenv(key); value != "" {
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return fallback
}
