// Package repository encapsulates the data access layer.
package repository

import (
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// McpRepo provides data access for MCP configurations.
type McpRepo struct {
	db *gorm.DB
}

// NewMcpRepo returns a new McpRepo.
func NewMcpRepo(db *gorm.DB) *McpRepo {
	return &McpRepo{db: db}
}

// List returns all MCP services.
func (r *McpRepo) List() ([]model.McpServer, error) {
	var list []model.McpServer
	err := r.db.Order("id").Find(&list).Error
	return list, err
}

// Create creates a server.
func (r *McpRepo) Create(m *model.McpServer) error {
	return r.db.Create(m).Error
}

// Update updates a server.
func (r *McpRepo) Update(m *model.McpServer) error {
	return r.db.Save(m).Error
}

// Delete deletes a server.
func (r *McpRepo) Delete(id uint) error {
	return r.db.Delete(&model.McpServer{}, id).Error
}
