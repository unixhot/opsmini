// Package service provides the business logic layer.
package service

import (
	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// AlertService handles alert rule business logic.
type AlertService struct {
	repo *repository.AlertRuleRepo
}

// NewAlertService creates an AlertService.
func NewAlertService(repo *repository.AlertRuleRepo) *AlertService {
	return &AlertService{repo: repo}
}

// List returns all rules.
func (s *AlertService) List() ([]model.AlertRule, error) {
	return s.repo.List()
}

// Create creates a rule.
func (s *AlertService) Create(rule *model.AlertRule) error {
	return s.repo.Create(rule)
}

// Update updates a rule.
func (s *AlertService) Update(rule *model.AlertRule) error {
	return s.repo.Update(rule)
}

// Delete deletes a rule.
func (s *AlertService) Delete(id uint) error {
	return s.repo.Delete(id)
}
