package model

import "time"

// AISession is a persisted AI Agent conversation.
type AISession struct {
	ID        string    `gorm:"primaryKey;size:40" json:"id"`
	UserID    uint      `gorm:"index" json:"user_id"`
	Title     string    `gorm:"size:200" json:"title"`
	Steps     int       `gorm:"default:0" json:"steps"`      // agent steps consumed in total
	ToolsUsed int       `gorm:"default:0" json:"tools_used"` // tool invocations in total
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AIMessage is one persisted conversation turn, including tool calls and tool
// results, so a suspended run can be resumed exactly where it stopped.
type AIMessage struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	SessionID  string    `gorm:"index;size:40" json:"session_id"`
	Seq        int       `gorm:"index" json:"seq"`
	Role       string    `gorm:"size:16" json:"role"` // system / user / assistant / tool
	Content    string    `gorm:"type:text" json:"content"`
	ToolCalls  string    `gorm:"type:text" json:"tool_calls"` // JSON array of tool calls
	ToolCallID string    `gorm:"size:64" json:"tool_call_id"`
	Name       string    `gorm:"size:64" json:"name"`
	CreatedAt  time.Time `json:"created_at"`
}

// AIPendingApproval is a tool call suspended waiting for human approval
// (the human-in-the-loop interrupt record).
type AIPendingApproval struct {
	ID         string    `gorm:"primaryKey;size:40" json:"id"`
	SessionID  string    `gorm:"index;size:40" json:"session_id"`
	UserID     uint      `gorm:"index" json:"user_id"`
	ToolName   string    `gorm:"size:64" json:"tool_name"`
	ToolCallID string    `gorm:"size:64" json:"tool_call_id"`
	Args       string    `gorm:"type:text" json:"args"` // decoded arguments as JSON
	Risk       int       `json:"risk"`
	Preview    string    `gorm:"type:text" json:"preview"`
	Status     string    `gorm:"size:16;index" json:"status"` // pending / approved / rejected / expired
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

// AI session message roles.
const (
	AIRoleSystem    = "system"
	AIRoleUser      = "user"
	AIRoleAssistant = "assistant"
	AIRoleTool      = "tool"
)

// Approval statuses.
const (
	ApprovalPending  = "pending"
	ApprovalApproved = "approved"
	ApprovalRejected = "rejected"
	ApprovalExpired  = "expired"
)
