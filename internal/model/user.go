// Package model defines GORM data models.
package model

import "time"

// Role constants, corresponding to the three RBAC roles.
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleReadonly = "readonly"
)

// User is a panel user.
type User struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	Username     string     `gorm:"uniqueIndex;size:64;not null" json:"username"`
	PasswordHash string     `gorm:"size:128" json:"-"`
	Role         string     `gorm:"size:32;default:operator" json:"role"`
	Nickname     string     `gorm:"size:64" json:"nickname"` // display name
	Icon         string     `gorm:"size:64" json:"icon"`     // avatar emoji / icon
	Email        string     `gorm:"size:128" json:"email"`   // contact email
	Status       int        `gorm:"default:1" json:"status"` // 1 enabled, 0 disabled
	LastLogin    *time.Time `json:"last_login,omitempty"`
	MFAEnabled   bool       `gorm:"default:false" json:"mfa_enabled"` // whether MFA is bound
	MFASecret    string     `gorm:"size:64" json:"-"`                 // TOTP secret (base32)
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
