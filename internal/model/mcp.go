// Package model defines GORM data models.
package model

import "time"

// McpServer is an MCP (Model Context Protocol) service configuration.
// MCP provides external tools and context for the AI assistant (e.g. filesystem, database, browser, etc.).
type McpServer struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;not null" json:"name"`     // name, e.g. filesystem
	Transport string    `gorm:"size:16;default:stdio" json:"transport"` // stdio / sse / http
	Command   string    `gorm:"size:256" json:"command"`          // stdio mode: launch command, e.g. npx -y @modelcontextprotocol/server-filesystem
	URL       string    `gorm:"size:256" json:"url"`              // sse/http mode: service address
	Enabled   bool      `gorm:"default:true" json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
