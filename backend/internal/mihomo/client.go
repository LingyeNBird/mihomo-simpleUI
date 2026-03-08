package mihomo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"mihomo-webui-proxy/backend/internal/model"
)

type Client struct {
	baseURL string
	secret  string
	http    *http.Client
}

type VersionResponse struct {
	Version string `json:"version"`
}

type proxiesResponse struct {
	Proxies map[string]proxyItem `json:"proxies"`
}

type proxyItem struct {
	Name    string        `json:"name"`
	Type    string        `json:"type"`
	Now     string        `json:"now"`
	All     []string      `json:"all"`
	History []model.Delay `json:"history"`
}

func NewClient(baseURL, secret string, timeout time.Duration) *Client {
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), secret: secret, http: &http.Client{Timeout: timeout}}
}

func (c *Client) GetVersion(ctx context.Context) (string, error) {
	var payload VersionResponse
	if err := c.doJSON(ctx, http.MethodGet, "/version", nil, &payload); err != nil {
		return "", err
	}
	return payload.Version, nil
}

func (c *Client) GetSelectableGroups(ctx context.Context) ([]model.ProxyGroup, error) {
	var payload proxiesResponse
	if err := c.doJSON(ctx, http.MethodGet, "/proxies", nil, &payload); err != nil {
		return nil, err
	}
	groups := make([]model.ProxyGroup, 0)
	for _, proxy := range payload.Proxies {
		if len(proxy.All) == 0 {
			continue
		}
		switch strings.ToLower(proxy.Type) {
		case "selector", "urltest", "fallback", "loadbalance":
			groups = append(groups, model.ProxyGroup{Name: proxy.Name, Type: proxy.Type, Current: proxy.Now, All: proxy.All, History: proxy.History})
		}
	}
	return groups, nil
}

func (c *Client) SelectNode(ctx context.Context, groupName string, nodeName string) error {
	return c.doJSON(ctx, http.MethodPut, "/proxies/"+url.PathEscape(groupName), map[string]string{"name": nodeName}, nil)
}

func (c *Client) ReloadConfig(ctx context.Context, configPath string) error {
	return c.doJSON(ctx, http.MethodPut, "/configs?force=true", map[string]string{"path": configPath, "payload": ""}, nil)
}

func (c *Client) doJSON(ctx context.Context, method string, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request body: %w", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	if c.secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.secret)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request mihomo controller: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mihomo controller returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
