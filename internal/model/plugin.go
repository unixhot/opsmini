package model

import "time"

// Plugin is one installed plugin. The authoritative state lives in
// <data>/plugins/state.json (so a wiped DB does not orphan running processes),
// and this table is the queryable mirror the panel reads.
type Plugin struct {
	ID         string    `gorm:"primaryKey;size:64" json:"id"`
	Name       string    `gorm:"size:128" json:"name"`
	Version    string    `gorm:"size:32" json:"version"`
	Vendor     string    `gorm:"size:128" json:"vendor"`
	Kind       string    `gorm:"size:16" json:"kind"`   // collector | action | panel
	State      string    `gorm:"size:16" json:"state"`  // installed | running | stopped | failed
	Enabled    bool      `json:"enabled"`
	Entry      string    `gorm:"size:256" json:"entry"`
	Dir        string    `gorm:"size:512" json:"dir"`
	LastError  string    `gorm:"size:512" json:"last_error"`
	Restarts   int       `json:"restarts"`
	InstalledAt time.Time `json:"installed_at"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
}
