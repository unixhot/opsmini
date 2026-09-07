// Package model defines GORM data models.
package model

import "time"

// Alert event status.
const (
	AlertStatusActive   = "active"   // active (unresolved)
	AlertStatusResolved = "resolved" // resolved
)

// Alert event kind.
const (
	AlertKindAlert    = "alert"    // alert triggered
	AlertKindRecovery = "recovery" // alert recovered
)

// AlertEvent is an alert event (triggered by alert rules; supports automatic cleanup after the retention period).
// An "alert" event is created on trigger; a "recovery" event (kind=recovery) is created when the alert clears,
// linked back to the alert event via RelatedID so the pair shows when the alert fired and when it recovered.
type AlertEvent struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	RuleID     uint       `json:"rule_id"`
	RuleName   string     `gorm:"size:128" json:"rule_name"`
	Metric     string     `gorm:"size:64" json:"metric"`  // CPU usage / memory usage / disk usage
	Level      string     `gorm:"size:16;default:warning" json:"level"`
	Message    string     `gorm:"size:256" json:"message"`
	Kind       string     `gorm:"size:16;default:alert" json:"kind"` // alert / recovery
	RelatedID  uint       `json:"related_id"`                        // for recovery event: the alert event it resolves
	Status     string     `gorm:"size:16;default:active" json:"status"` // active / resolved
	CreatedAt  time.Time  `json:"created_at"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
