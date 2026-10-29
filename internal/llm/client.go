package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Settings returns the live configuration, so panel changes take effect without a
// restart.
type Settings func() Config

// Client dispatches one logical call to the right wire protocol.
type Client struct {
	http *http.Client
	cfg  Settings
}

// New creates a client. cfg is evaluated on every call.
func New(cfg Settings) *Client {
	if cfg == nil {
		cfg = func() Config { return Config{} }
	}
	return &Client{
		// Long timeout: a reasoning model on a slow relay can legitimately take
		// minutes, and cutting it off produces a confusing half-answer.
		http: &http.Client{Timeout: 10 * time.Minute},
		cfg:  cfg,
	}
}

// Config returns the current normalized configuration.
func (c *Client) Config() Config { return c.cfg().Normalized() }

// Validate reports configuration problems before a request is attempted.
func (c *Client) Validate() error {
	cfg := c.Config()
	if cfg.BaseURL == "" {
		return errors.New("AI 接口地址未配置")
	}
	if cfg.Model == "" {
		return errors.New("AI 模型未配置")
	}
	// Gemini can authenticate with an API key in the query string, and local
	// deployments (Ollama, vLLM) need no key at all.
	if cfg.APIKey == "" && cfg.Provider != ProviderGemini && !isLocalEndpoint(cfg.BaseURL) {
		return errors.New("AI API Key 未配置")
	}
	return nil
}

// isLocalEndpoint reports whether the base URL points at this machine or a private
// network, where an API key usually does not exist.
func isLocalEndpoint(base string) bool {
	for _, p := range []string{"http://127.0.0.1", "http://localhost", "http://0.0.0.0", "http://[::1]"} {
		if strings.HasPrefix(base, p) {
			return true
		}
	}
	return false
}

// Chat performs a non-streaming completion.
func (c *Client) Chat(ctx context.Context, msgs []Message, tools []Tool) (*Message, *Usage, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	cfg := c.Config()
	switch cfg.Provider {
	case ProviderAnthropic:
		return c.chatAnthropic(ctx, cfg, msgs, tools)
	case ProviderGemini:
		return c.chatGemini(ctx, cfg, msgs, tools)
	default:
		return c.chatOpenAI(ctx, cfg, msgs, tools)
	}
}

// ChatStream performs one streaming completion. onDelta receives incremental
// assistant text. The returned Message holds the full content plus every tool call.
func (c *Client) ChatStream(ctx context.Context, msgs []Message, tools []Tool, onDelta func(string)) (*Message, *Usage, error) {
	if err := c.Validate(); err != nil {
		return nil, nil, err
	}
	cfg := c.Config()
	switch cfg.Provider {
	case ProviderAnthropic:
		return c.streamAnthropic(ctx, cfg, msgs, tools, onDelta)
	case ProviderGemini:
		return c.streamGemini(ctx, cfg, msgs, tools, onDelta)
	default:
		return c.streamOpenAI(ctx, cfg, msgs, tools, onDelta)
	}
}

// httpError turns a non-2xx response into a readable error. Providers put the
// useful detail in the body, so it is included but truncated.
func httpError(provider string, resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	detail := truncate(strings.TrimSpace(string(body)), 400)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("%s 鉴权失败（HTTP %d）：检查 API Key 与协议类型是否匹配 — %s", provider, resp.StatusCode, detail)
	}
	return fmt.Errorf("%s 接口返回 %d：%s", provider, resp.StatusCode, detail)
}

// truncateText keeps the first n bytes, appending an ellipsis.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

var _ = fmt.Sprint
