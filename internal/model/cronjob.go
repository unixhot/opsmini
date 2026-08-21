package model

import "time"

// Cron job source constants.
const (
	CronKindPanel  = "panel"  // panel-managed jobs (stored in SQLite)
	CronKindSystem = "system" // system crontab (/etc/crontab etc.)
	CronKindUser   = "user"   // user crontab (/var/spool/cron/<user>)
)

// CronJob is a cron job.
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
}
