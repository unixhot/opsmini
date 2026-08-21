// Package service provides the business logic layer.
package service

import (
	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// McpService handles MCP configuration business logic.
type McpService struct {
	repo *repository.McpRepo
}

// NewMcpService creates an McpService.
func NewMcpService(repo *repository.McpRepo) *McpService {
	return &McpService{repo: repo}
}

// List returns all MCP servers.
func (s *McpService) List() ([]model.McpServer, error) {
	return s.repo.List()
}

// Create creates an MCP server.
func (s *McpService) Create(m *model.McpServer) error {
	return s.repo.Create(m)
}

// Update updates an MCP server.
func (s *McpService) Update(m *model.McpServer) error {
	return s.repo.Update(m)
}

// Delete deletes an MCP server.
func (s *McpService) Delete(id uint) error {
	return s.repo.Delete(id)
}
