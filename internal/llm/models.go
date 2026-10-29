package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Model listing.
//
// Why this exists: with a hosted vendor the model name is a public constant, but
// with a local server it is whatever the operator pulled (`qwen2.5:7b-instruct-q4_K_M`)
// or whatever `--served-model-name` was passed to vLLM. Asking the user to paste a
// name they have to look up in a shell is the single most common way to get a first
// request wrong, so the panel asks the endpoint instead.

type modelEntry struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type modelList struct {
	Data   []modelEntry `json:"data"`
	Models []modelEntry `json:"models"`
}

// ListModels asks the configured endpoint which models it offers.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	cfg := c.Config()
	if err := validateEndpoint(cfg); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	var (
		names []string
		err   error
	)
	switch Protocol(cfg.Provider) {
	case ProviderAnthropic:
		names, err = c.listAnthropicModels(ctx, cfg)
	case ProviderGemini:
		names, err = c.listGeminiModels(ctx, cfg)
	default:
		names, err = c.listOpenAIStyle(ctx, cfg)
		// Ollama older than 0.1.30 only lists models at its native /api/tags.
		if err != nil && cfg.Provider == ProviderOllama {
			if alt, altErr := c.listOllamaTags(ctx, cfg); altErr == nil {
				names, err = alt, nil
			}
		}
	}
	if err != nil {
		return nil, err
	}

	seen := make(map[string]bool, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil, fmt.Errorf("服务返回了空模型列表")
	}
	return out, nil
}

// listOpenAIStyle covers OpenAI, DeepSeek, Ollama's /v1 shim, vLLM and relays.
func (c *Client) listOpenAIStyle(ctx context.Context, cfg Config) ([]string, error) {
	var body modelList
	if err := c.getJSON(ctx, cfg.BaseURL+"/models", c.bearer(cfg), &body); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(body.Data))
	for _, m := range body.Data {
		names = append(names, m.ID)
	}
	return names, nil
}

// listOllamaTags hits the native endpoint, which lives next to /v1 rather than
// under it: http://host:11434/api/tags.
func (c *Client) listOllamaTags(ctx context.Context, cfg Config) ([]string, error) {
	root := strings.TrimSuffix(cfg.BaseURL, "/v1")
	var body modelList
	if err := c.getJSON(ctx, root+"/api/tags", nil, &body); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(body.Models))
	for _, m := range body.Models {
		if m.Name != "" {
			names = append(names, m.Name)
			continue
		}
		names = append(names, m.ID)
	}
	return names, nil
}

func (c *Client) listAnthropicModels(ctx context.Context, cfg Config) ([]string, error) {
	hdr := map[string]string{"anthropic-version": "2023-06-01"}
	if cfg.APIKey != "" {
		hdr["x-api-key"] = cfg.APIKey
	}
	var body modelList
	if err := c.getJSON(ctx, cfg.BaseURL+"/v1/models", hdr, &body); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(body.Data))
	for _, m := range body.Data {
		names = append(names, m.ID)
	}
	return names, nil
}

func (c *Client) listGeminiModels(ctx context.Context, cfg Config) ([]string, error) {
	url := cfg.BaseURL + "/v1beta/models?pageSize=200"
	if cfg.APIKey != "" {
		url += "&key=" + cfg.APIKey
	}
	var body modelList
	if err := c.getJSON(ctx, url, nil, &body); err != nil {
		return nil, err
	}
	// Gemini returns "models/gemini-2.5-pro"; the call path wants the bare id.
	names := make([]string, 0, len(body.Models))
	for _, m := range body.Models {
		name := m.Name
		if name == "" {
			name = m.ID
		}
		names = append(names, strings.TrimPrefix(name, "models/"))
	}
	return names, nil
}

// bearer returns the auth header for OpenAI-compatible endpoints. An empty key is
// fine for Ollama and vLLM, which usually run without auth.
func (c *Client) bearer(cfg Config) map[string]string {
	if cfg.APIKey == "" {
		return nil
	}
	return map[string]string{"Authorization": "Bearer " + cfg.APIKey}
}

func (c *Client) getJSON(ctx context.Context, url string, hdr map[string]string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("请求地址不合法：%w", err)
	}
	req.Header.Set("Accept", "application/json")
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("连接 %s 失败：%w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return httpError("AI 接口", resp)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("读取响应失败：%w", err)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("响应不是预期的 JSON：%w", err)
	}
	return nil
}
