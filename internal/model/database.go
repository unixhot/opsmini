package model

import "time"

// Database is a database instance.
type Database struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null" json:"name"`
	Type      string    `gorm:"size:16;default:mysql" json:"type"` // mysql/postgresql
	Charset   string    `gorm:"size:32;default:utf8mb4" json:"charset"`
	Status    int       `gorm:"default:1" json:"status"` // 1 running, 0 stopped
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
