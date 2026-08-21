package service

import (
	"log"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// AuditService handles audit logging.
type AuditService struct {
	repo *repository.AuditLogRepo
}

// NewAuditService creates an AuditService.
func NewAuditService(repo *repository.AuditLogRepo) *AuditService {
	return &AuditService{repo: repo}
}

// Record writes an audit log entry (asynchronous, does not block the main flow; failures are only logged).
func (s *AuditService) Record(userID uint, username, action, resource, detail, ip string) {
	entry := &model.AuditLog{
		UserID:   userID,
		Username: username,
		Action:   action,
		Resource: resource,
		Detail:   truncate(detail, 500),
		IP:       ip,
	}
	if err := s.repo.Create(entry); err != nil {
		log.Printf("audit record failed: %v", err)
	}
}

// List returns the most recent limit audit log entries.
func (s *AuditService) List(limit int) ([]model.AuditLog, error) {
	return s.repo.List(limit)
}
