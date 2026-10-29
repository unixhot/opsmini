package aiagent

import (
	"context"

	"github.com/opsmini/opsmini/internal/llm"
)

// Model transport for the agent.
//
// The wire protocols live in internal/llm; this file is only the adaptor between
// the agent's own types and the neutral ones, so the agent loop stays free of
// provider details and adding a provider never touches it.
//
// Supported providers (see internal/llm): OpenAI-compatible (OpenAI, DeepSeek,
// Qwen, Moonshot, Zhipu, Ollama, vLLM, one-api relays), Anthropic/Claude,
// Azure OpenAI, Google Gemini.

// LLMConfig is a resolved model endpoint configuration.
type LLMConfig struct {
	// Provider is one of llm.Provider*; empty means OpenAI-compatible.
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
	// MaxTokens caps the reply. Anthropic requires it.
	MaxTokens int
	// AzureAPIVersion is only used by the azure provider.
	AzureAPIVersion string
}

// Ready reports whether the configuration is usable.
func (c LLMConfig) Ready() bool {
	return c.BaseURL != "" && c.Model != ""
}

// toLLM converts to the transport config.
func (c LLMConfig) toLLM() llm.Config {
	return llm.Config{
		Provider:        c.Provider,
		BaseURL:         c.BaseURL,
		APIKey:          c.APIKey,
		Model:           c.Model,
		MaxTokens:       c.MaxTokens,
		AzureAPIVersion: c.AzureAPIVersion,
	}
}

// LLMSettings returns the live model configuration, so panel settings take effect
// without restarting the process.
type LLMSettings func() LLMConfig

// LLMClient is the agent's model client.
type LLMClient struct {
	inner *llm.Client
	cfg   LLMSettings
}

// NewLLMClient creates a client. cfg is evaluated on every call.
func NewLLMClient(cfg LLMSettings) *LLMClient {
	if cfg == nil {
		cfg = func() LLMConfig { return LLMConfig{} }
	}
	return &LLMClient{
		inner: llm.New(func() llm.Config { return cfg().toLLM() }),
		cfg:   cfg,
	}
}

// Provider reports the active provider identifier.
func (c *LLMClient) Provider() string { return c.inner.Config().Provider }

// Validate reports configuration problems before a request is attempted.
func (c *LLMClient) Validate() error { return c.inner.Validate() }

// Chat performs a non-streaming completion.
func (c *LLMClient) Chat(ctx context.Context, msgs []Message, tools []Tool) (*Message, *Usage, error) {
	msg, usage, err := c.inner.Chat(ctx, toLLMMessages(msgs), toLLMTools(tools))
	return fromLLMMessage(msg), fromLLMUsage(usage), err
}

// ChatStream performs one streaming completion. onDelta receives incremental
// assistant text (may be nil).
func (c *LLMClient) ChatStream(ctx context.Context, msgs []Message, tools []Tool, onDelta func(string)) (*Message, *Usage, error) {
	msg, usage, err := c.inner.ChatStream(ctx, toLLMMessages(msgs), toLLMTools(tools), onDelta)
	return fromLLMMessage(msg), fromLLMUsage(usage), err
}

// --- conversions -----------------------------------------------------------

func toLLMMessages(in []Message) []llm.Message {
	out := make([]llm.Message, 0, len(in))
	for _, m := range in {
		lm := llm.Message{
			Role:       m.Role,
			Content:    m.Content,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		for _, tc := range m.ToolCalls {
			var c llm.ToolCall
			c.ID = tc.ID
			c.Type = tc.Type
			c.Function.Name = tc.Function.Name
			c.Function.Arguments = tc.Function.Arguments
			lm.ToolCalls = append(lm.ToolCalls, c)
		}
		out = append(out, lm)
	}
	return out
}

func toLLMTools(in []Tool) []llm.Tool {
	out := make([]llm.Tool, 0, len(in))
	for _, t := range in {
		out = append(out, llm.Tool{Name: t.Name, Description: t.Description, Schema: t.Schema})
	}
	return out
}

func fromLLMMessage(m *llm.Message) *Message {
	if m == nil {
		return nil
	}
	out := &Message{Role: m.Role, Content: m.Content, ToolCallID: m.ToolCallID, Name: m.Name}
	for _, tc := range m.ToolCalls {
		var c ToolCall
		c.ID = tc.ID
		c.Type = tc.Type
		c.Function.Name = tc.Function.Name
		c.Function.Arguments = tc.Function.Arguments
		out.ToolCalls = append(out.ToolCalls, c)
	}
	return out
}

func fromLLMUsage(u *llm.Usage) *Usage {
	if u == nil {
		return nil
	}
	return &Usage{PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens, TotalTokens: u.TotalTokens}
}

// truncateText keeps the first n bytes with an ellipsis. Used by memory folding
// and report generation as well as the transport, so it stays here.
func truncateText(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}
