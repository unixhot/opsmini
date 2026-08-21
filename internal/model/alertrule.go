// Package model defines GORM data models.
package model

import "time"

// AlertRule is an alert rule (monitoring alert configuration).
type AlertRule struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:128;not null" json:"name"`
	Metric    string    `gorm:"size:64" json:"metric"`    // CPU usage / memory usage / disk usage / service status
	Condition string    `gorm:"size:64" json:"condition"` // > 90% / > 85% / process exit
	Duration  string    `gorm:"size:32" json:"duration"`  // duration, e.g. 5 minutes
	Notify    string    `gorm:"size:128" json:"notify"`   // notification method, e.g. email + panel
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
