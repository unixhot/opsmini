// Package aiagent implements OpsMini's built-in AI Agent: a lightweight ReAct
// (Reason → Act → Observe) kernel with tool calling, risk-based guardrails and
// human-in-the-loop confirmation.
//
// Design notes:
//   - Tools are declared with plain JSON Schema (MCP compatible) instead of a
//     framework-specific format, so the agent is never locked into a particular
//     framework and the very same registry can be re-exposed as an MCP server.
//   - Every tool carries a risk level: L0 (read-only) runs unattended, L1
//     (low-risk write) is configurable, L2 (high-risk) always requires an
//     explicit human approval, and L3 command patterns are hard-forbidden.
package aiagent

import "encoding/json"

// RiskLevel classifies how dangerous a tool is.
type RiskLevel int

const (
	// RiskReadonly is a read-only operation (L0): runs without confirmation.
	RiskReadonly RiskLevel = iota
	// RiskLowWrite is a low-risk write operation (L1): runs without confirmation
	// only when the administrator explicitly opts the tool in.
	RiskLowWrite
	// RiskHighWrite is a high-risk write operation (L2): always requires an
	// explicit human confirmation before execution.
	RiskHighWrite
	// RiskForbidden is hard-blocked (L3): never executed, not even with approval.
	RiskForbidden
)

// String returns the stable identifier used by the API and the frontend.
func (r RiskLevel) String() string {
	switch r {
	case RiskReadonly:
		return "readonly"
	case RiskLowWrite:
		return "low_write"
	case RiskHighWrite:
		return "high_write"
	case RiskForbidden:
		return "forbidden"
	}
	return "unknown"
}

// Tool is a single capability the agent can invoke. Schema is a JSON Schema
// object, the same shape MCP uses for a tool's inputSchema.
type Tool struct {
	Name        string         `json:"name"`
	Title       string         `json:"title,omitempty"`
	Description string         `json:"description"`
	Schema      map[string]any `json:"schema"`
	Risk        RiskLevel      `json:"risk"`
	// Perm is the panel permission required to invoke the tool. An empty value
	// means the tool only needs the generic ai.agent permission.
	Perm string `json:"perm,omitempty"`
	// Preview renders a human-readable summary of what the call will do. It is
	// shown in the confirmation dialog and recorded in the audit trail.
	Preview func(args map[string]any) string `json:"-"`
	// Execute runs the tool.
	Execute func(ctx ExecContext, args map[string]any) (Result, error) `json:"-"`
	// Source is "builtin" for OpsMini's own tools and "mcp:<server>" for tools
	// proxied from an external MCP server.
	Source string `json:"source,omitempty"`
}

// Result is what a tool returns.
type Result struct {
	// Summary is the compact form fed back to the model (keeps the context small).
	Summary string `json:"summary"`
	// Detail is the full output shown to the user (may be much longer).
	Detail string `json:"detail,omitempty"`
	// OK reports whether the tool succeeded.
	OK bool `json:"ok"`
}

// Message is an OpenAI-compatible chat message. Keeping our own copy (instead of
// reusing service.ChatMessage) keeps this package free of service dependencies.
type Message struct {
	Role       string     `json:"role"` // system / user / assistant / tool
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// ToolCall is a model-requested tool invocation.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"` // raw JSON string as produced by the model
	} `json:"function"`
}

// Args decodes the raw arguments into a map. Malformed JSON yields an empty map
// rather than an error, so a bad model output cannot break the loop.
func (t ToolCall) Args() map[string]any {
	out := map[string]any{}
	if t.Function.Arguments == "" {
		return out
	}
	if err := json.Unmarshal([]byte(t.Function.Arguments), &out); err != nil {
		return map[string]any{}
	}
	return out
}

// Usage reports token consumption of a single LLM call when the provider sends it.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// --- OpenAI wire format ---

// toolDef is the OpenAI function-calling tool declaration.
type toolDef struct {
	Type     string  `json:"type"`
	Function funcDef `json:"function"`
}

// funcDef is the OpenAI function declaration.
type funcDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}
