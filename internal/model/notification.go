package model

import "time"

// Notification levels.
const (
	NotifLevelInfo  = "info"  // info
	NotifLevelWarn  = "warn"  // warning
	NotifLevelError = "error" // error
)

// Notification is a panel notification (produced by alert triggers, system events, etc.; supports read/delete management).
type Notification struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Level     string    `gorm:"size:16;default:info" json:"level"` // info / warn / error
	Title     string    `gorm:"size:128" json:"title"`
	Content   string    `gorm:"size:512" json:"content"`
	Read      bool      `gorm:"default:false" json:"read"`
	CreatedAt time.Time `json:"created_at"`
}
