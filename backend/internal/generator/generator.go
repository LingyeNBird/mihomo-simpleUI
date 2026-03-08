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
	Name      string   `yaml:"name"`
	Type      string   `yaml:"type"`
	Proxies   []string `yaml:"proxies,omitempty"`
	Use       []string `yaml:"use,omitempty"`
	URL       string   `yaml:"url,omitempty"`
	Interval  int      `yaml:"interval,omitempty"`
	Tolerance int      `yaml:"tolerance,omitempty"`
}

func Generate(subscriptions []model.Subscription, selections []model.Selection, opts Options) (Generated, error) {
	selectionMap := make(map[string]string, len(selections))
	for _, selection := range selections {
		selectionMap[selection.GroupName] = selection.NodeName
	}

	proxyProviders := make(map[string]map[string]any)
	providerSet := make([]string, 0, len(subscriptions))
	enabledNodeNames := make([]string, 0, len(subscriptions)+1)
	inlineProxies := make([]map[string]any, 0)
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
		proxyNames, proxies, err := loadSubscriptionProxies(sub.FilePath)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("subscription %s could not load proxies: %v", sub.Name, err))
			continue
		}
		if len(proxyNames) == 0 {
			warnings = append(warnings, fmt.Sprintf("subscription %s does not contain usable proxies", sub.Name))
			continue
		}
		proxyProviders[providerName] = map[string]any{
			"type": "file",
			"path": filepath.ToSlash(sub.FilePath),
			"health-check": map[string]any{
				"enable": true, "url": "https://www.gstatic.com/generate_204", "interval": 300,
			},
		}
		providerSet = append(providerSet, providerName)
		enabledNodeNames = append(enabledNodeNames, proxyNames...)
		inlineProxies = append(inlineProxies, proxies...)
	}

	manualProxyChoices := append([]string{"DIRECT"}, enabledNodeNames...)
	proxyGroups := []proxyGroup{
		{Name: "Proxy", Type: "select", Proxies: providerAndBuiltin(manualProxyChoices, selectionMap["Proxy"])},
	}
	if len(providerSet) > 0 {
		proxyGroups = append(proxyGroups, proxyGroup{Name: "Auto", Type: "url-test", Use: providerSet, URL: "https://www.gstatic.com/generate_204", Interval: 300, Tolerance: 50})
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
		"proxies":         inlineProxies,
		"proxy-providers": proxyProviders,
		"proxy-groups":    proxyGroups,
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

func providerDisplayName(sub model.Subscription) string {
	name := strings.TrimSpace(sub.Name)
	if name == "" {
		return fmt.Sprintf("subscription-%d", sub.ID)
	}
	return name
}

func loadSubscriptionProxies(filePath string) ([]string, []map[string]any, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, nil, err
	}
	var payload map[string]any
	if err := yaml.Unmarshal(content, &payload); err != nil {
		return nil, nil, err
	}
	rawProxies, ok := payload["proxies"]
	if !ok {
		return nil, nil, nil
	}
	items, ok := rawProxies.([]any)
	if !ok {
		return nil, nil, fmt.Errorf("invalid proxies payload")
	}
	names := make([]string, 0, len(items))
	proxies := make([]map[string]any, 0, len(items))
	for _, item := range items {
		proxy, ok := item.(map[string]any)
		if !ok {
			continue
		}
		name, _ := proxy["name"].(string)
		if strings.TrimSpace(name) == "" {
			continue
		}
		names = append(names, name)
		proxies = append(proxies, proxy)
	}
	return names, proxies, nil
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
