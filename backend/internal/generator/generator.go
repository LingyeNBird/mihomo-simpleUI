package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"mihomo-webui-proxy/backend/internal/model"
)

type Options struct {
	MixedPort      int
	ControllerPort int
	Secret         string
	Mode           string
	ConfigPath     string
}

type Generated struct {
	Warnings []string
}

type proxyGroup struct {
	Name    string   `yaml:"name"`
	Type    string   `yaml:"type"`
	Proxies []string `yaml:"proxies"`
}

func Generate(subscriptions []model.Subscription, selections []model.Selection, opts Options) (Generated, error) {
	selectionMap := make(map[string]string, len(selections))
	for _, selection := range selections {
		selectionMap[selection.GroupName] = selection.NodeName
	}

	proxyProviders := make(map[string]map[string]any)
	proxySet := make([]string, 0, len(subscriptions))
	warnings := make([]string, 0)
	sort.Slice(subscriptions, func(i, j int) bool { return subscriptions[i].ID < subscriptions[j].ID })

	for _, sub := range subscriptions {
		if !sub.Enabled {
			continue
		}
		if sub.FilePath == "" {
			warnings = append(warnings, fmt.Sprintf("subscription %s has no file path", sub.Name))
			continue
		}
		providerName := providerKey(sub)
		proxyProviders[providerName] = map[string]any{
			"type": "file",
			"path": filepath.ToSlash(sub.FilePath),
			"health-check": map[string]any{
				"enable": true, "url": "https://www.gstatic.com/generate_204", "interval": 300,
			},
		}
		proxySet = append(proxySet, providerName)
	}

	config := map[string]any{
		"mixed-port":          opts.MixedPort,
		"allow-lan":           true,
		"bind-address":        "0.0.0.0",
		"mode":                opts.Mode,
		"log-level":           "info",
		"external-controller": fmt.Sprintf("0.0.0.0:%d", opts.ControllerPort),
		"secret":              opts.Secret,
		"profile":             map[string]any{"store-selected": true, "store-fake-ip": true},
		"dns": map[string]any{
			"enable": true, "listen": "0.0.0.0:1053", "ipv6": false, "enhanced-mode": "fake-ip",
			"nameserver": []string{"1.1.1.1", "8.8.8.8"},
			"fallback":   []string{"https://1.1.1.1/dns-query", "https://8.8.8.8/dns-query"},
		},
		"proxy-providers": proxyProviders,
		"proxy-groups": []proxyGroup{
			{Name: "Proxy", Type: "select", Proxies: providerAndBuiltin(proxySet, selectionMap["Proxy"])},
			{Name: "Auto", Type: "url-test", Proxies: providerAndBuiltin(proxySet, selectionMap["Auto"])},
		},
		"rules": []string{"MATCH,Proxy"},
	}

	payload, err := yaml.Marshal(config)
	if err != nil {
		return Generated{}, fmt.Errorf("marshal generated config: %w", err)
	}
	if err := writeAtomically(opts.ConfigPath, payload); err != nil {
		return Generated{}, fmt.Errorf("write generated config: %w", err)
	}
	return Generated{Warnings: warnings}, nil
}

func providerKey(sub model.Subscription) string {
	slug := strings.TrimSpace(strings.ToLower(sub.Name))
	if slug == "" {
		slug = fmt.Sprintf("subscription-%d", sub.ID)
	}
	slug = strings.NewReplacer(" ", "-", "/", "-", "\\", "-").Replace(slug)
	return fmt.Sprintf("%s-%d", slug, sub.ID)
}

func providerAndBuiltin(providers []string, preferred string) []string {
	values := append([]string{"DIRECT"}, providers...)
	if preferred == "" {
		return values
	}
	for _, current := range values {
		if current == preferred {
			return values
		}
	}
	return values
}

func writeAtomically(target string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}
