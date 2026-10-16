package model

import "time"

// Cron job source/kind constants.
//
// Panel tasks run shell commands in-process; system/user tasks are read-only
// views of the system crontab; agent tasks hand a natural-language prompt to the
// AI Agent and archive the resulting report.
const (
	CronKindPanel  = "panel"  // panel-managed shell jobs (stored in SQLite)
	CronKindSystem = "system" // system crontab (/etc/crontab etc.)
	CronKindUser   = "user"   // user crontab (/var/spool/cron/<user>)
	CronKindAgent  = "agent"  // AI Agent jobs (prompt + tools, unattended)
)

// Cron execution outcomes.
const (
	CronStatusSuccess = "success"
	CronStatusFailed  = "failed"
)

// CronJob is a scheduled task.
type CronJob struct {
	ID        uint       `gorm:"primaryKey" json:"id"`
	Name      string     `gorm:"size:128;not null" json:"name"`
	Kind      string     `gorm:"size:16;default:panel" json:"kind"`
	User      string     `gorm:"size:32" json:"user"`
	Schedule  string     `gorm:"size:64" json:"schedule"` // standard 5-field cron expression
	Command   string     `gorm:"size:512" json:"command"`
	Enabled   bool       `gorm:"default:true" json:"enabled"`
	LastRun   *time.Time `json:"last_run,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`

	// --- AI Agent tasks (kind=agent) ---

	// Prompt is the natural-language instruction handed to the agent.
	Prompt string `gorm:"type:text" json:"prompt"`
	// Notify lists the notification channels for the report, comma-separated
	// labels (面板 / 邮件 / 企业微信 / 钉钉 / 飞书). Empty means "面板".
	Notify string `gorm:"size:128" json:"notify"`
	// MaxSteps caps the ReAct iterations for this task (0 = server default).
	MaxSteps int `json:"max_steps"`
	// TimeoutSeconds caps the whole run (0 = 600).
	TimeoutSeconds int `json:"timeout_seconds"`
	// SkippedTools lists the tool calls refused during the last run because they
	// needed human approval and the task runs unattended.
	SkippedTools string `gorm:"size:512" json:"skipped_tools"`
	// LastSession is the AI session holding the report of the last run.
	LastSession string `gorm:"size:40" json:"last_session"`
	// LastReport is the archived HTML report of the last run, relative to the
	// reports directory (e.g. "每日主机巡检/每日主机巡检-20261016-140000.html").
	// It is what the task list links to for "open" and "download".
	LastReport string `gorm:"size:255" json:"last_report"`
	// LastStatus / LastError summarise the last run for the task list.
	LastStatus string `gorm:"size:16" json:"last_status"`
	LastError  string `gorm:"size:512" json:"last_error"`
	// FailCount counts consecutive failures; the task auto-disables at the limit.
	FailCount int `json:"fail_count"`
}
