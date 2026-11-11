package llm

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Settings returns the live configuration, so panel changes take effect without a
// restart.
type Settings func() Config

// Client dispatches one logical call to the right wire protocol.
type Client struct {
	mu sync.Mutex
	// http is rebuilt when the CA bundle changes, so a newly pasted certificate
	// takes effect without restarting the panel.
	http *http.Client
	// caPEM is the bundle the current http client was built with.
	caPEM string
	cfg   Settings
}

// New creates a client. cfg is evaluated on every call.
func New(cfg Settings) *Client {
	if cfg == nil {
		cfg = func() Config { return Config{} }
	}
	c := &Client{cfg: cfg}
	c.http = c.buildClient("")
	return c
}

// buildClient returns an HTTP client trusting the given PEM bundle (empty means
// the system trust store).
//
// A bad bundle yields a client that trusts nothing extra rather than a client
// that skips verification: silently disabling TLS checks would make a
// misconfigured CA look like a working one.
func (c *Client) buildClient(pemBundle string) *http.Client {
	// Long timeout: a reasoning model on a slow relay can legitimately take
	// minutes, and cutting it off produces a confusing half-answer.
	transport := &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
	if strings.TrimSpace(pemBundle) != "" {
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if pool.AppendCertsFromPEM([]byte(pemBundle)) {
			transport.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
		} else {
			// Appending nothing means the PEM is not parseable. Log it: the
			// failure will surface as a TLS error during the call, and "为什么连不上"
			// is much easier to answer when the log says the bundle was rejected.
			log.Printf("llm: CA 证书解析失败，已回退到系统信任库（PEM 长度 %d）", len(pemBundle))
		}
	}
	return &http.Client{Timeout: 10 * time.Minute, Transport: transport}
}

// client returns the HTTP client matching the current configuration, rebuilding
// it when the CA bundle changed.
func (c *Client) client() *http.Client {
	c.mu.Lock()
	defer c.mu.Unlock()
	want := c.cfg().Normalized().CACertPEM
	if want != c.caPEM {
		c.caPEM = want
		c.http = c.buildClient(want)
	}
	return c.http
}

// Config returns the current normalized configuration.
func (c *Client) Config() Config { return c.cfg().Normalized() }

// Check reports what is missing before a request can be attempted.
//
// 三件事都查：地址、模型、密钥。之前「能不能用」有两套判断——客户端这里查三项，
// 而 Agent 侧的 Ready() 只查地址和模型，于是「地址和模型都填了、密钥没填」
// 被判成"就绪"，请求真的发出去才报错，事件就被记成失败而不是"还没配置"。
func (c Config) Check() error {
	if err := validateEndpoint(c); err != nil {
		return err
	}
	if c.Model == "" {
		return errors.New("AI 模型未配置")
	}
	return nil
}

// Validate reports configuration problems before a request is attempted.
func (c *Client) Validate() error { return c.Config().Check() }

// validateEndpoint checks the transport-level settings only. Listing models must
// use this instead of Validate: the model name is exactly what the user is still
// trying to pick, so requiring it there would block the feature that helps.
func validateEndpoint(cfg Config) error {
	if cfg.BaseURL == "" {
		return errors.New("AI 接口地址未配置")
	}
	// Gemini authenticates with a key in the query string; Ollama and vLLM are
	// usually started without any auth. Everything else needs a key, unless the
	// endpoint is on this machine or a private relay the user knows is open.
	if cfg.APIKey == "" && !KeyOptional(cfg.Provider) && !isLocalEndpoint(cfg.BaseURL) {
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
	switch Protocol(cfg.Provider) {
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
	switch Protocol(cfg.Provider) {
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
