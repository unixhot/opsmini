package model

import "time"

// Website is a website site.
type Website struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	Domain     string    `gorm:"size:255;not null" json:"domain"`
	Dir        string    `gorm:"size:512" json:"dir"`
	Runtime    string    `gorm:"size:32;default:php" json:"runtime"` // php/node/static
	PHPVersion string    `gorm:"size:16" json:"php_version"`
	SSL        bool      `json:"ssl"`
	Status     int       `gorm:"default:1" json:"status"` // 1 running, 0 stopped
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}
