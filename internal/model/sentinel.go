// Package model defines GORM data models.
package model

import "time"

// Signal severity levels.
const (
	SeverityInfo     = "info"
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"
)

// SeverityRank maps a severity name to a comparable rank.
func SeverityRank(s string) int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	}
	return 0
}

// Signal kinds (one per collector family).
const (
	SignalKindMetric = "metric"
	SignalKindLog    = "log"
	SignalKindAuth   = "auth"
	SignalKindFile   = "file"
	SignalKindProc   = "proc"
	SignalKindNet    = "net"
)

// Incident lifecycle.
const (
	IncidentOpen      = "open"       // signals aggregated, no agent run yet
	IncidentTriggered = "triggered"  // a policy matched, run queued
	IncidentRunning   = "running"    // agent session in progress
	IncidentResolved  = "resolved"   // verification passed (or human marked)
	IncidentIgnored   = "ignored"    // suppressed by a human
	IncidentTakenOver = "taken_over" // a human took over; the agent must not act
	IncidentFailed    = "failed"     // run failed / verification failed
)

// Trigger modes.
const (
	ModeShadow = "shadow" // investigate and record intent, never write
	ModeAssist = "assist" // read automatically, writes need approval
	ModeAuto   = "auto"   // whitelisted low-risk writes run automatically
)

// ActionLog outcomes.
const (
	ActionAllow   = "allow"
	ActionConfirm = "confirm"
	ActionDeny    = "deny"
	ActionShadow  = "shadow"
)

// Signal is one normalized observation produced by a collector.
//
// Fingerprint is the normalized identity of the observation ("auth.new_ip_login",
// "log_growth:/var/log/app.log") and is what dedupe and incident aggregation key
// on. Evidence holds a small JSON blob plus a pointer to a file on disk when the
// payload is large.
type Signal struct {
	ID          string    `gorm:"primaryKey;size:32" json:"id"`
	Kind        string    `gorm:"size:16;index" json:"kind"`
	Source      string    `gorm:"size:64" json:"source"`
	Subject     string    `gorm:"size:256;index" json:"subject"`
	Severity    string    `gorm:"size:16;index" json:"severity"`
	Fingerprint string    `gorm:"size:128;index" json:"fingerprint"`
	Family      string    `gorm:"size:64;index" json:"family"`
	DedupKey    string    `gorm:"size:192;index" json:"dedup_key"`
	Message     string    `gorm:"size:512" json:"message"`
	Deviation   float64   `json:"deviation"`
	Evidence    string    `gorm:"type:text" json:"evidence"`
	Tags        string    `gorm:"size:256" json:"tags"`
	IncidentID  string    `gorm:"size:32;index" json:"incident_id"`
	ObservedAt  time.Time `json:"observed_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// Incident is an aggregated cluster of related signals plus the agent run it
// triggered (if any).
type Incident struct {
	ID          string     `gorm:"primaryKey;size:32" json:"id"`
	Title       string     `gorm:"size:192" json:"title"`
	Family      string     `gorm:"size:64;index" json:"family"`
	Subject     string     `gorm:"size:256" json:"subject"`
	Severity    string     `gorm:"size:16;index" json:"severity"`
	Status      string     `gorm:"size:16;index" json:"status"`
	Count       int        `json:"count"`
	FirstSeen   time.Time  `json:"first_seen"`
	LastSeen    time.Time  `json:"last_seen"`
	PolicyID    uint       `json:"policy_id"`
	PolicyName  string     `gorm:"size:96" json:"policy_name"`
	Mode        string     `gorm:"size:16" json:"mode"`
	SessionID   string     `gorm:"size:32" json:"session_id"`
	ReportRel   string     `gorm:"size:256" json:"report_rel"`
	Summary     string     `gorm:"type:text" json:"summary"`
	VerifyState string     `gorm:"size:24" json:"verify_state"`
	IgnoredBy   string     `gorm:"size:64" json:"ignored_by"`
	IgnoreNote  string     `gorm:"size:256" json:"ignore_note"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	ClosedAt    *time.Time `json:"closed_at,omitempty"`
}

// TriggerPolicy decides whether an incident wakes the agent, in which mode, with
// which tool subset and with which budget.
type TriggerPolicy struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `gorm:"size:96;uniqueIndex" json:"name"`
	Enabled     bool      `gorm:"default:true" json:"enabled"`
	Family      string    `gorm:"size:64" json:"family"`
	MinSeverity string    `gorm:"size:16;default:high" json:"min_severity"`
	Mode        string    `gorm:"size:16;default:shadow" json:"mode"`
	CooldownSec int       `gorm:"default:1800" json:"cooldown_sec"`
	MaxWrites   int       `gorm:"default:2" json:"max_writes"`
	MaxSteps    int       `gorm:"default:12" json:"max_steps"`
	MaxSeconds  int       `gorm:"default:300" json:"max_seconds"`
	MaxTokens   int       `gorm:"default:150000" json:"max_tokens"`
	AllowTools  string    `gorm:"type:text" json:"allow_tools"`
	DenyTools   string    `gorm:"type:text" json:"deny_tools"`
	Verify      string    `gorm:"type:text" json:"verify"`
	// AutoTools lists the L1 tools allowed to run without confirmation when the
	// policy mode is "auto". Everything not listed still needs a human.
	AutoTools   string    `gorm:"type:text" json:"auto_tools"`
	Notify      string    `gorm:"size:128" json:"notify"`
	Prompt      string    `gorm:"type:text" json:"prompt"`
	Description string    `gorm:"size:256" json:"description"`
	LastRunAt   *time.Time `json:"last_run_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// AgentRun records one event-triggered agent execution.
type AgentRun struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	IncidentID string     `gorm:"size:32;index" json:"incident_id"`
	PolicyName string     `gorm:"size:96" json:"policy_name"`
	Mode       string     `gorm:"size:16" json:"mode"`
	Status     string     `gorm:"size:16" json:"status"` // running | success | failed | skipped
	SessionID  string     `gorm:"size:32" json:"session_id"`
	Steps      int        `json:"steps"`
	Tools      int        `json:"tools"`
	Writes     int        `json:"writes"`
	Skipped    string     `gorm:"type:text" json:"skipped"`
	Summary    string     `gorm:"type:text" json:"summary"`
	ReportRel  string     `gorm:"size:256" json:"report_rel"`
	Error      string     `gorm:"size:512" json:"error"`
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

// ActionLog is the decision chain: every tool call the agent attempted, the
// policy verdict that allowed/denied it, and the result.
type ActionLog struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	IncidentID string    `gorm:"size:32;index" json:"incident_id"`
	SessionID  string    `gorm:"size:32;index" json:"session_id"`
	StepNo     int       `json:"step_no"`
	Tool       string    `gorm:"size:64;index" json:"tool"`
	Args       string    `gorm:"type:text" json:"args"`
	Risk       string    `gorm:"size:16" json:"risk"`
	Verdict    string    `gorm:"size:16;index" json:"verdict"`
	Rule       string    `gorm:"size:128" json:"rule"`
	Result     string    `gorm:"type:text" json:"result"`
	Reversible bool      `json:"reversible"`
	RollbackID string    `gorm:"size:64" json:"rollback_id"`
	DurationMS int64     `json:"duration_ms"`
	CreatedAt  time.Time `json:"created_at"`
}

// QuarantineItem is a file moved out of the way instead of being deleted. The
// original path, checksum and mode are kept so it can be restored verbatim.
type QuarantineItem struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	IncidentID   string     `gorm:"size:32;index" json:"incident_id"`
	OriginalPath string     `gorm:"size:512" json:"original_path"`
	StoredPath   string     `gorm:"size:512" json:"stored_path"`
	SHA256       string     `gorm:"size:64" json:"sha256"`
	Size         int64      `json:"size"`
	Mode         uint32     `json:"mode"`
	Owner        string     `gorm:"size:64" json:"owner"`
	Reason       string     `gorm:"size:256" json:"reason"`
	RestoredAt   *time.Time `json:"restored_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

// SnapshotRecord is a pre-change copy of a file or config.
type SnapshotRecord struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	IncidentID  string     `gorm:"size:32;index" json:"incident_id"`
	Path        string     `gorm:"size:512" json:"path"`
	StoredPath  string     `gorm:"size:512" json:"stored_path"`
	Kind        string     `gorm:"size:16" json:"kind"` // file | config
	SHA256      string     `gorm:"size:64" json:"sha256"`
	Size        int64      `json:"size"`
	RolledBack  bool       `json:"rolled_back"`
	RolledBackAt *time.Time `json:"rolled_back_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// Baseline stores learned expectations (EWMA per hour bucket, seen IPs, ...).
type Baseline struct {
	Key        string    `gorm:"primaryKey;size:192" json:"key"`
	Kind       string    `gorm:"size:32" json:"kind"`
	Value      float64   `json:"value"`
	Sigma      float64   `json:"sigma"`
	Samples    int       `json:"samples"`
	Extra      string    `gorm:"type:text" json:"extra"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// SignalFeedback records a human verdict on a signal (used to tune thresholds).
type SignalFeedback struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SignalID   string    `gorm:"size:32;index" json:"signal_id"`
	Fingerprint string   `gorm:"size:128;index" json:"fingerprint"`
	Verdict    string    `gorm:"size:24" json:"verdict"` // false_positive | true_positive
	Note       string    `gorm:"size:256" json:"note"`
	By         string    `gorm:"size:64" json:"by"`
	CreatedAt  time.Time `json:"created_at"`
}

// BlockedIP is a temporary firewall ban with a TTL so a wrong block heals itself.
type BlockedIP struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	IP        string     `gorm:"size:64;index" json:"ip"`
	Reason    string     `gorm:"size:256" json:"reason"`
	IncidentID string    `gorm:"size:32;index" json:"incident_id"`
	Source    string     `gorm:"size:16" json:"source"` // agent | manual
	ExpiresAt time.Time  `gorm:"index" json:"expires_at"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}
