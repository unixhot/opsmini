// Package model defines GORM data models.
package model

import "time"

// App is a software store application (application template). Each app maps to a set of Shell scripts,
// which manage services via Docker internally (one-click install comparable to the 1Panel app store).
// Supports import/export: exported as a zip (README.md + logo.png + scripts/*.sh + files/).
type App struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	Slug          string    `gorm:"uniqueIndex;size:64;not null" json:"slug"` // unique identifier, e.g. redis
	Name          string    `gorm:"size:64;not null" json:"name"`             // display name, e.g. Redis
	Description   string    `gorm:"size:512" json:"description"`              // description
	Category      string    `gorm:"size:32;default:other" json:"category"`    // category key
	Icon          string    `gorm:"size:8" json:"icon"`                       // short identifier (fallback when logo.png is absent)
	Color         string    `gorm:"size:16" json:"color"`                     // theme color
	Version       string    `gorm:"size:64" json:"version"`                   // default image tag
	Ports         string    `gorm:"size:128" json:"ports"`                    // port description, e.g. 6379:6379
	ContainerName string    `gorm:"size:64" json:"container_name"`            // container name (used to determine install status)
	README        string    `gorm:"type:text" json:"readme"`                  // README.md content
	InstallScript string    `gorm:"type:text" json:"install_script"`          // scripts/install.sh
	UpgradeScript string    `gorm:"type:text" json:"upgrade_script"`          // scripts/upgrade.sh
	RemoveScript  string    `gorm:"type:text" json:"remove_script"`           // scripts/remove.sh
	HasLogo       bool      `gorm:"default:false" json:"has_logo"`            // whether logo.png exists
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}
