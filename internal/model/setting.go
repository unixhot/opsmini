// Package model defines GORM data models.
package model

import "time"

// Setting is a key-value panel setting (persisted runtime-changeable config, e.g. metrics token).
type Setting struct {
	Key       string    `gorm:"primaryKey;size:64" json:"key"`
	Value     string    `gorm:"size:512" json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}
