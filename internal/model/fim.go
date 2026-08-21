// Package model defines GORM data models.
package model

import "time"

// FimBaseline is a file integrity monitoring baseline (SHA256 snapshot of critical files).
type FimBaseline struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Path      string    `gorm:"size:256;uniqueIndex" json:"path"`
	SHA256    string    `gorm:"size:64" json:"sha256"`
	Size      int64     `json:"size"`
	ModTime   int64     `json:"mod_time"`
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FimChange is a file integrity change event (critical file tampered/removed/added).
type FimChange struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Path      string    `gorm:"size:256" json:"path"`
	OldSHA256 string    `gorm:"size:64" json:"old_sha256"`
	NewSHA256 string    `gorm:"size:64" json:"new_sha256"`
	Size      int64     `json:"size"`
	ModTime   int64     `json:"mod_time"`
	Level     string    `gorm:"size:16;default:warning" json:"level"` // warning/critical
	Message   string    `gorm:"size:256" json:"message"`
	CreatedAt time.Time `json:"created_at"`
}
