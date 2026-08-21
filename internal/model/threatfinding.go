// Package model defines GORM data models.
package model

import "time"

// ThreatFinding is a threat finding (suspicious process/cron job/startup item).
type ThreatFinding struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	Kind       string     `gorm:"size:32;index" json:"kind"` // process/cron/startup
	Name       string     `gorm:"size:128" json:"name"`
	Detail     string     `gorm:"type:text" json:"detail"`
	Severity   string     `gorm:"size:16;default:warning" json:"severity"` // info/warning/critical
	Status     string     `gorm:"size:16;default:active" json:"status"`    // active/resolved
	FirstSeen  time.Time  `json:"first_seen"`
	LastSeen   time.Time  `json:"last_seen"`
	ResolvedAt *time.Time `json:"resolved_at,omitempty"`
}
