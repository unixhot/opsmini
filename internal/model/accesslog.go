// Package model defines GORM data models.
package model

import "time"

// AccessLog is a panel HTTP access log (records each API request).
type AccessLog struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Method    string    `gorm:"size:8" json:"method"`
	Path      string    `gorm:"size:256" json:"path"`
	Status    int       `json:"status"`
	IP        string    `gorm:"size:64" json:"ip"`
	LatencyMs int64     `json:"latency_ms"`
	Username  string    `gorm:"size:64" json:"username"`
	CreatedAt time.Time `json:"created_at"`
}
