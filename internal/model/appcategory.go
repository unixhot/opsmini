// Package model defines GORM data models.
package model

import "time"

// AppCategory is an application category. Software store apps link to categories via category (storing the category Key).
type AppCategory struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"size:32;uniqueIndex" json:"key"` // unique identifier: web/database/middleware/other
	Name      string    `gorm:"size:64" json:"name"`            // display name: website/database/middleware/other
	Builtin   bool      `gorm:"default:false" json:"builtin"`   // whether it is a built-in category
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BuiltinCategories returns the four built-in categories.
func BuiltinCategories() []AppCategory {
	return []AppCategory{
		{Key: "web", Name: "网站", Builtin: true},
		{Key: "database", Name: "数据库", Builtin: true},
		{Key: "middleware", Name: "中间件", Builtin: true},
		{Key: "other", Name: "其它", Builtin: true},
	}
}
