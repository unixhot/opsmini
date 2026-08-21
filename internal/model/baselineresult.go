// Package model defines GORM data models.
package model

import "time"

// BaselineResult is a security baseline check result (one batch per scan; grouped by ScanID).
type BaselineResult struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	ScanID     string    `gorm:"size:32;index" json:"scan_id"` // same batch number within one scan
	Category   string    `gorm:"size:32" json:"category"`      // ssh/password/account/file_perm/firewall/cron/systemd
	ItemKey    string    `gorm:"size:64" json:"item_key"`      // unique identifier of the check item
	Title      string    `gorm:"size:128" json:"title"`
	Status     string    `gorm:"size:16" json:"status"` // pass/fail/warn
	Detail     string    `gorm:"type:text" json:"detail"`
	Suggestion string    `gorm:"type:text" json:"suggestion"`
	Weight     int       `json:"weight"`
	CreatedAt  time.Time `json:"created_at"`
}
