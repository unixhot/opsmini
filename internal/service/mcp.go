// Package service provides the business logic layer.
package service

import (
	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// McpService handles MCP configuration business logic.
type McpService struct {
	repo *repository.McpRepo
	// OnChange is called after any write so the live MCP connections can be
	// reconciled immediately. Without it a newly added server stayed
	// disconnected until the next background reload (up to 2 minutes), which is
	// indistinguishable from "the feature does not work".
	OnChange func()
}

// NewMcpService creates an McpService.
func NewMcpService(repo *repository.McpRepo) *McpService {
	return &McpService{repo: repo}
}

func (s *McpService) changed() {
	if s.OnChange != nil {
		s.OnChange()
	}
}

// List returns all MCP servers.
func (s *McpService) List() ([]model.McpServer, error) {
	return s.repo.List()
}

// Create creates an MCP server.
func (s *McpService) Create(m *model.McpServer) error {
	if err := s.repo.Create(m); err != nil {
		return err
	}
	s.changed()
	return nil
}

// Update updates an MCP server.
func (s *McpService) Update(m *model.McpServer) error {
	if err := s.repo.Update(m); err != nil {
		return err
	}
	s.changed()
	return nil
}

// Delete deletes an MCP server.
func (s *McpService) Delete(id uint) error {
	if err := s.repo.Delete(id); err != nil {
		return err
	}
	s.changed()
	return nil
}
