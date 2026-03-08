package subscription

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"mihomo-webui-proxy/backend/internal/model"
)

type Downloader struct {
	client *http.Client
	dir    string
}

type RefreshResult struct {
	Subscription model.Subscription `json:"subscription"`
	Warnings     []string           `json:"warnings,omitempty"`
}

type convertedProxy struct {
	Name           string `yaml:"name"`
	Type           string `yaml:"type"`
	Server         string `yaml:"server"`
	Port           int    `yaml:"port"`
	Cipher         string `yaml:"cipher,omitempty"`
	Password       string `yaml:"password,omitempty"`
	UUID           string `yaml:"uuid,omitempty"`
	AlterID        *int   `yaml:"alterId"`
	UDP            bool   `yaml:"udp,omitempty"`
	TLS            bool   `yaml:"tls,omitempty"`
	SkipCertVerify bool   `yaml:"skip-cert-verify,omitempty"`
	Network        string `yaml:"network,omitempty"`
	ServerName     string `yaml:"servername,omitempty"`
	WSOpts         any    `yaml:"ws-opts,omitempty"`
}

type vmessPayload struct {
	PS   string `json:"ps"`
	Add  string `json:"add"`
	Port string `json:"port"`
	ID   string `json:"id"`
	Aid  string `json:"aid"`
	Net  string `json:"net"`
	Type string `json:"type"`
	Host string `json:"host"`
	Path string `json:"path"`
	TLS  string `json:"tls"`
	SNI  string `json:"sni"`
}

var unsafeNamePattern = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func NewDownloader(timeout time.Duration, dir string) *Downloader {
	return &Downloader{client: &http.Client{Timeout: timeout}, dir: dir}
}

func (d *Downloader) Download(ctx context.Context, item model.Subscription) (RefreshResult, error) {
	parsed, err := url.Parse(item.URL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return RefreshResult{}, fmt.Errorf("invalid subscription url")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("User-Agent", "mihomo-webui-proxy/0.1")
	resp, err := d.client.Do(req)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("download subscription: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return RefreshResult{}, fmt.Errorf("subscription responded with status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return RefreshResult{}, fmt.Errorf("read subscription body: %w", err)
	}
	content := strings.TrimPrefix(string(body), "\ufeff")
	warnings := []string(nil)
	content, warnings, err = prepareSubscriptionContent(content)
	if err != nil {
		return RefreshResult{}, err
	}
	if err := os.MkdirAll(d.dir, 0o755); err != nil {
		return RefreshResult{}, fmt.Errorf("ensure subscriptions dir: %w", err)
	}
	if item.FilePath == "" {
		item.FilePath = filepath.Join(d.dir, fmt.Sprintf("subscription-%d.yaml", item.ID))
	}
	if err := writeAtomically(item.FilePath, []byte(content)); err != nil {
		return RefreshResult{}, fmt.Errorf("write subscription file: %w", err)
	}
	now := time.Now().UTC()
	item.LastRefreshedAt = &now
	item.LastError = ""
	return RefreshResult{Subscription: item, Warnings: warnings}, nil
}

func FilePathFor(dir string, name string, id int64) string {
	safeName := strings.Trim(strings.ToLower(name), " ")
	safeName = unsafeNamePattern.ReplaceAllString(safeName, "-")
	if safeName == "" {
		safeName = fmt.Sprintf("subscription-%d", id)
	}
	return filepath.Join(dir, fmt.Sprintf("%s-%d.yaml", safeName, id))
}

func prepareSubscriptionContent(content string) (string, []string, error) {
	if normalized, converted, err := normalizeSubscriptionContent(content); err != nil {
		return "", nil, err
	} else {
		content = normalized
		if converted {
			return content, []string{"subscription content was converted from Clash URI list to Mihomo YAML"}, validateYAMLMapping(content)
		}
	}
	return content, nil, validateYAMLMapping(content)
}

func validateYAMLMapping(content string) error {
	if strings.TrimSpace(content) == "" {
		return fmt.Errorf("subscription content is empty")
	}
	var payload map[string]any
	if err := yaml.Unmarshal([]byte(content), &payload); err != nil {
		return fmt.Errorf("invalid yaml: %w", err)
	}
	if _, ok := payload["proxies"]; !ok {
		if _, providers := payload["proxy-providers"]; !providers {
			return fmt.Errorf("subscription must contain proxies or proxy-providers")
		}
	}
	return nil
}

func normalizeSubscriptionContent(content string) (string, bool, error) {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "proxies:") || strings.Contains(trimmed, "\nproxy-providers:") {
		return content, false, nil
	}

	decoded, err := base64.StdEncoding.DecodeString(removeWhitespace(trimmed))
	if err != nil {
		decoded, err = base64.RawStdEncoding.DecodeString(removeWhitespace(trimmed))
	}
	if err != nil {
		return content, false, nil
	}

	decodedText := strings.TrimSpace(string(decoded))
	if decodedText == "" {
		return "", false, fmt.Errorf("subscription content is empty after decoding")
	}
	if !strings.Contains(decodedText, "://") {
		return content, false, nil
	}

	uriLines := make([]string, 0)
	for _, line := range strings.Split(decodedText, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		uriLines = append(uriLines, line)
	}
	if len(uriLines) == 0 {
		return "", false, fmt.Errorf("decoded subscription does not contain URI entries")
	}

	proxies := make([]convertedProxy, 0, len(uriLines))
	proxyMaps := make([]map[string]any, 0, len(uriLines))
	for index, line := range uriLines {
		proxy, ok := parseSubscriptionURI(line, index)
		if ok {
			proxies = append(proxies, proxy)
			proxyMaps = append(proxyMaps, convertedProxyToMap(proxy))
		}
	}
	if len(proxies) == 0 {
		return "", false, fmt.Errorf("decoded subscription did not yield any supported vmess/ss nodes")
	}

	payload := map[string]any{
		"proxies": proxyMaps,
		"proxy-groups": []map[string]any{{
			"name":    "Proxy",
			"type":    "select",
			"proxies": append([]string{"DIRECT"}, collectProxyNames(proxies)...),
		}},
		"rules": []string{"MATCH,Proxy"},
	}

	output, err := yaml.Marshal(payload)
	if err != nil {
		return "", false, fmt.Errorf("marshal converted subscription yaml: %w", err)
	}
	return string(output), true, nil
}

func collectProxyNames(proxies []convertedProxy) []string {
	items := make([]string, 0, len(proxies))
	for _, proxy := range proxies {
		items = append(items, proxy.Name)
	}
	return items
}

func convertedProxyToMap(proxy convertedProxy) map[string]any {
	result := map[string]any{
		"name":   proxy.Name,
		"type":   proxy.Type,
		"server": proxy.Server,
		"port":   proxy.Port,
		"udp":    proxy.UDP,
	}
	if proxy.AlterID != nil {
		result["alterId"] = *proxy.AlterID
	}
	if proxy.Cipher != "" {
		result["cipher"] = proxy.Cipher
	}
	if proxy.Password != "" {
		result["password"] = proxy.Password
	}
	if proxy.UUID != "" {
		result["uuid"] = proxy.UUID
	}
	if proxy.TLS {
		result["tls"] = true
	}
	if proxy.SkipCertVerify {
		result["skip-cert-verify"] = true
	}
	if proxy.Network != "" {
		result["network"] = proxy.Network
	}
	if proxy.ServerName != "" {
		result["servername"] = proxy.ServerName
	}
	if proxy.WSOpts != nil {
		result["ws-opts"] = proxy.WSOpts
	}
	return result
}

func removeWhitespace(value string) string {
	return strings.NewReplacer("\n", "", "\r", "", "\t", "", " ", "").Replace(value)
}

func parseSubscriptionURI(line string, index int) (convertedProxy, bool) {
	switch {
	case strings.HasPrefix(line, "vmess://"):
		return parseVMess(line, index)
	case strings.HasPrefix(line, "ss://"):
		return parseSS(line, index)
	default:
		return convertedProxy{}, false
	}
}

func parseVMess(line string, index int) (convertedProxy, bool) {
	payload := strings.TrimPrefix(strings.TrimSpace(line), "vmess://")
	decoded, err := decodeBase64String(payload)
	if err != nil {
		return convertedProxy{}, false
	}
	text := strings.TrimSpace(string(decoded))
	if !strings.HasPrefix(text, "{") {
		text = strings.TrimLeft(text, "/")
		if nested, nestedErr := decodeBase64String(text); nestedErr == nil {
			text = strings.TrimSpace(string(nested))
		}
	}

	var item vmessPayload
	if err := json.Unmarshal([]byte(text), &item); err != nil {
		return convertedProxy{}, false
	}
	port, err := strconv.Atoi(item.Port)
	if err != nil || item.Add == "" || item.ID == "" {
		return convertedProxy{}, false
	}
	proxy := convertedProxy{
		Name:   firstNonEmpty(item.PS, fmt.Sprintf("vmess-%d", index+1)),
		Type:   "vmess",
		Server: item.Add,
		Port:   port,
		Cipher: "auto",
		UUID:   item.ID,
		UDP:    true,
	}
	aidValue := 0
	if aid, err := strconv.Atoi(item.Aid); err == nil {
		aidValue = aid
	}
	proxy.AlterID = &aidValue
	if strings.TrimSpace(item.Net) != "" {
		proxy.Network = item.Net
	}
	if strings.TrimSpace(item.TLS) != "" && item.TLS != "none" {
		proxy.TLS = true
		proxy.ServerName = firstNonEmpty(item.SNI, item.Host)
	}
	if proxy.Network == "ws" {
		headers := map[string]string{}
		if strings.TrimSpace(item.Host) != "" {
			headers["Host"] = item.Host
		}
		wsOpts := map[string]any{}
		if strings.TrimSpace(item.Path) != "" {
			wsOpts["path"] = item.Path
		}
		if len(headers) > 0 {
			wsOpts["headers"] = headers
		}
		if len(wsOpts) > 0 {
			proxy.WSOpts = wsOpts
		}
	}
	return proxy, true
}

func parseSS(line string, index int) (convertedProxy, bool) {
	body := strings.TrimPrefix(strings.TrimSpace(line), "ss://")
	name := fmt.Sprintf("ss-%d", index+1)
	if hashIndex := strings.Index(body, "#"); hashIndex >= 0 {
		if decodedName, err := url.PathUnescape(body[hashIndex+1:]); err == nil && decodedName != "" {
			name = decodedName
		}
		body = body[:hashIndex]
	}
	var userInfo string
	var hostPort string
	if atIndex := strings.LastIndex(body, "@"); atIndex >= 0 {
		userInfo = body[:atIndex]
		hostPort = body[atIndex+1:]
		if decoded, err := decodeBase64String(userInfo); err == nil && strings.Contains(string(decoded), ":") {
			userInfo = string(decoded)
		}
	} else {
		decoded, err := decodeBase64String(body)
		if err != nil {
			return convertedProxy{}, false
		}
		parts := strings.SplitN(string(decoded), "@", 2)
		if len(parts) != 2 {
			return convertedProxy{}, false
		}
		userInfo, hostPort = parts[0], parts[1]
	}
	credentials := strings.SplitN(userInfo, ":", 2)
	hostParts := strings.Split(hostPort, ":")
	if len(credentials) != 2 || len(hostParts) < 2 {
		return convertedProxy{}, false
	}
	port, err := strconv.Atoi(hostParts[len(hostParts)-1])
	if err != nil {
		return convertedProxy{}, false
	}
	server := strings.Join(hostParts[:len(hostParts)-1], ":")
	if server == "" {
		return convertedProxy{}, false
	}
	return convertedProxy{
		Name:     name,
		Type:     "ss",
		Server:   server,
		Port:     port,
		Cipher:   credentials[0],
		Password: credentials[1],
		UDP:      true,
	}, true
}

func decodeBase64String(value string) ([]byte, error) {
	cleaned := removeWhitespace(value)
	if mod := len(cleaned) % 4; mod != 0 {
		cleaned += strings.Repeat("=", 4-mod)
	}
	if decoded, err := base64.StdEncoding.DecodeString(cleaned); err == nil {
		return decoded, nil
	}
	return base64.RawStdEncoding.DecodeString(removeWhitespace(value))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func writeAtomically(target string, content []byte) error {
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, content, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}
