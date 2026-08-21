package model

import "time"

// AuditLog is an audit log.
type AuditLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	UserID    uint      `json:"user_id"`
	Username  string    `gorm:"size:64" json:"username"`
	Action    string    `gorm:"size:32" json:"action"`   // login / login_fail / create / update / delete / command
	Resource  string    `gorm:"size:64" json:"resource"` // user / website / database / cron / file / system
	Detail    string    `gorm:"size:512" json:"detail"`
	IP        string    `gorm:"size:64" json:"ip"`
	CreatedAt time.Time `json:"created_at"`
}
