// Package model defines GORM data models.
package model

import "time"

// Alert event status.
const (
	AlertStatusActive   = "active"   // active (unresolved)
	AlertStatusResolved = "resolved" // resolved
)

// AlertEvent is an alert event (triggered by alert rules; supports automatic cleanup after the retention period).
type AlertEvent struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	RuleID     uint       `json:"rule_id"`
	RuleName   string     `gorm:"size:128" json:"rule_name"`
	Metric     string     `gorm:"size:64" json:"metric"`  // CPU usage / memory usage / disk usage
	Level      string     `gorm:"size:16;default:warning" json:"level"`
	Message    string     `gorm:"size:256" json:"message"`
	Status     string     `gorm:"size:16;default:active" json:"status"` // active / resolved
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
