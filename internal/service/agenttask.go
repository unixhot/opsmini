package service

import (
	"context"
	"time"
)

// AgentTask describes one unattended AI Agent task run.
//
// Scheduled tasks execute with nobody watching, so the agent must never wait for
// a human approval. The implementation refuses every tool that requires
// confirmation and reports it back in Result.Skipped.
type AgentTask struct {
	// ID / Name identify the scheduled task (used for the report title).
	ID   uint
	Name string
	// Prompt is the natural-language instruction.
	Prompt string
	// MaxSteps caps the ReAct iterations (0 = engine default).
	MaxSteps int
	// Timeout caps the whole run (0 = implementation default).
	Timeout time.Duration
}

// AgentTaskResult is the outcome of an unattended AI task run.
type AgentTaskResult struct {
	// SessionID is the AI session that holds the report.
	SessionID string
	// Title is the session title (shown in the AI assistant sidebar).
	Title string
	// Content is the final answer — the report body.
	Content string
	// Steps / Tools count the reasoning steps and executed tool calls.
	Steps int
	Tools int
	// Skipped lists the tool calls refused because they need human approval.
	Skipped []string
	// ReportPath is the archived Markdown report on disk (may be empty).
	ReportPath string
	// Duration is the wall-clock time of the run.
	Duration time.Duration
}

// AgentRunner executes AI Agent tasks unattended. It is implemented by the
// aiagent layer and injected by the router, which keeps this package free of a
// dependency on the agent engine.
type AgentRunner interface {
	// RunUnattended runs the task and returns its report. A non-nil error means
	// the task failed; the result may still carry partial information.
	RunUnattended(ctx context.Context, task AgentTask) (AgentTaskResult, error)
}
