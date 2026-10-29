// Package llm is the model transport layer: one client, several wire protocols.
//
// Why a separate package instead of extending the agent's client: OpsMini talks to
// models from two places (the AI Agent loop and the panel assistant), and adding a
// provider used to mean touching both. The wire formats differ in ways that are not
// cosmetic — Anthropic puts the system prompt in its own field and returns content
// blocks, Gemini nests everything in contents/parts, Azure keeps OpenAI's body but
// changes both the URL shape and the auth header. Keeping those differences behind
// one interface means callers stay provider-agnostic.
//
// Supported today:
//
//	openai     /chat/completions  — OpenAI, DeepSeek, Qwen/DashScope, Moonshot,
//	                               Zhipu, Ollama, vLLM, one-api/new-api, ...
//	anthropic  /v1/messages       — Claude, and every Anthropic-compatible relay
//	azure      Azure OpenAI deployments (api-key header + deployment URL)
//	gemini     generateContent    — Google Gemini
package llm

import "strings"

// Provider identifiers (stored in settings, so they are stable strings).
//
// DeepSeek, Ollama and vLLM speak OpenAI's wire format but are kept as separate
// ids on purpose: the panel needs to prefill a different base URL, treat the API
// key as optional (both are commonly run without auth on localhost or a private
// network), and list models from a different place. Collapsing them into "openai"
// would force users to know all of that.
const (
	ProviderOpenAI    = "openai"
	ProviderDeepSeek  = "deepseek"
	ProviderOllama    = "ollama"
	ProviderVLLM      = "vllm"
	ProviderAnthropic = "anthropic"
	ProviderAzure     = "azure"
	ProviderGemini    = "gemini"
)

// KnownProviders lists the values the panel may send, in display order.
var KnownProviders = []string{
	ProviderOpenAI, ProviderDeepSeek, ProviderAnthropic, ProviderAzure, ProviderGemini,
	ProviderOllama, ProviderVLLM,
}

// Protocol maps a provider id to the wire protocol it speaks. Anything not listed
// explicitly is OpenAI-compatible, which is also the right answer for relays and
// for provider ids added by a newer panel build.
func Protocol(p string) string {
	switch p {
	case ProviderAnthropic:
		return ProviderAnthropic
	case ProviderAzure:
		return ProviderAzure
	case ProviderGemini:
		return ProviderGemini
	default: // openai, deepseek, ollama, vllm, relays
		return ProviderOpenAI
	}
}

// KeyOptional reports whether a missing API key is acceptable. Ollama and vLLM are
// routinely started without auth; Gemini accepts the key in the query string.
func KeyOptional(p string) bool {
	switch p {
	case ProviderOllama, ProviderVLLM, ProviderGemini:
		return true
	}
	return false
}

// Message is one turn of a conversation, provider-neutral.
type Message struct {
	Role string `json:"role"` // system / user / assistant / tool
	// Content is the text of the message. For an assistant turn it holds the
	// visible text; the tool calls live in ToolCalls.
	Content string `json:"content,omitempty"`
	// ToolCalls is populated on assistant turns that requested tools.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolCallID links a tool result back to the call it answers.
	ToolCallID string `json:"tool_call_id,omitempty"`
	// Name is the tool name on a tool result (Anthropic/Gemini need it; OpenAI
	// ignores it).
	Name string `json:"name,omitempty"`
}

// ToolCall is one model-requested invocation.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name string `json:"name"`
		// Arguments is the raw JSON string the model produced, exactly as received,
		// so callers can decide how to handle malformed output.
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// Tool is a capability offered to the model, declared with plain JSON Schema
// (the same shape MCP uses).
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
}

// Usage reports token consumption when the provider sends it.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Config is a resolved endpoint configuration.
type Config struct {
	Provider string
	BaseURL  string
	APIKey   string
	Model    string
	// MaxTokens is the reply cap. Anthropic requires this field, so a zero value
	// is replaced with a sane default per provider.
	MaxTokens int
	// AzureAPIVersion is only used by the azure provider.
	AzureAPIVersion string
}

// Normalized returns the config with provider-specific defaults applied.
func (c Config) Normalized() Config {
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	if c.Provider == "" {
		// OpenAI-compatible is the de-facto default: it covers OpenAI itself and
		// every relay that copied its shape.
		c.Provider = ProviderOpenAI
	}
	switch c.Provider {
	case ProviderAnthropic:
		// Anthropic rejects requests without max_tokens.
		if c.MaxTokens <= 0 {
			c.MaxTokens = 4096
		}
	case ProviderGemini:
		if c.MaxTokens <= 0 {
			c.MaxTokens = 8192
		}
	default:
		if c.MaxTokens <= 0 {
			c.MaxTokens = 4096
		}
	}
	if c.Provider == ProviderAzure && c.AzureAPIVersion == "" {
		c.AzureAPIVersion = "2024-10-21"
	}
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	return c
}

// Ready reports whether the configuration looks usable.
func (c Config) Ready() bool {
	return c.BaseURL != "" && c.Model != ""
}
