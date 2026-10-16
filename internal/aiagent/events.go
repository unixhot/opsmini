package aiagent

import "context"

// Event types emitted on the agent's SSE stream.
const (
	EventSession    = "session"     // the session was created/resolved (carries session_id)
	EventStep       = "step"        // a new reasoning step started
	EventThinking   = "thinking"    // incremental assistant text
	EventToolCall   = "tool_call"   // the model asked to run a tool
	EventToolResult = "tool_result" // a tool finished
	EventConfirm    = "confirm"     // a tool is suspended waiting for approval
	EventFinal      = "final"       // the final answer
	EventError      = "error"       // the run failed
	EventDone       = "done"        // the stream is complete
)

// Event is a single item on the agent's event stream.
type Event struct {
	Type      string         `json:"type"`
	SessionID string         `json:"session_id,omitempty"`
	Step      int            `json:"step,omitempty"`
	Delta     string         `json:"delta,omitempty"`
	ID        string         `json:"id,omitempty"` // tool_call_id
	Name      string         `json:"name,omitempty"`
	Title     string         `json:"title,omitempty"`
	Args      map[string]any `json:"args,omitempty"`
	Risk      string         `json:"risk,omitempty"`
	RiskCode  int            `json:"risk_code,omitempty"`
	Preview   string         `json:"preview,omitempty"`
	OK        *bool          `json:"ok,omitempty"`
	Summary   string         `json:"summary,omitempty"`
	Detail    string         `json:"detail,omitempty"`
	Duration  int64          `json:"duration_ms,omitempty"`

	// Confirmation payload.
	ApprovalID string `json:"approval_id,omitempty"`
	ExpiresAt  int64  `json:"expires_at,omitempty"`

	// Final answer / error payload.
	Content string `json:"content,omitempty"`
	Message string `json:"message,omitempty"`

	Usage   *Usage `json:"usage,omitempty"`
	Pending bool   `json:"pending,omitempty"`
}

// Emitter receives agent events. The loop emits from a single goroutine.
type Emitter interface {
	Emit(Event)
}

// EmitterFunc adapts a plain function to the Emitter interface.
type EmitterFunc func(Event)

// Emit implements Emitter.
func (f EmitterFunc) Emit(e Event) { f(e) }

// nopEmitter discards every event (used when the caller does not stream).
func nopEmitter() Emitter { return EmitterFunc(func(Event) {}) }

// ExecContext is what a tool receives when it runs. It carries the request
// context plus the identity needed for auditing and progress reporting.
type ExecContext struct {
	context.Context
	SessionID string
	UserID    uint
	Username  string
	// Emit lets a long-running tool stream progress to the user (may be nil).
	Emit func(Event)
}

// progress reports a human-readable progress line without creating a tool result.
func (c ExecContext) progress(msg string) {
	if c.Emit != nil {
		c.Emit(Event{Type: EventThinking, Delta: msg})
	}
}

// WithContext returns a copy of the execution context with a different context.Context.
func (c ExecContext) WithContext(ctx context.Context) ExecContext {
	c.Context = ctx
	return c
}
