// Package repository encapsulates the data access layer.
package repository

import (
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// AlertRuleRepo provides data access for alert rules.
type AlertRuleRepo struct {
	db *gorm.DB
}

// NewAlertRuleRepo returns a new AlertRuleRepo.
func NewAlertRuleRepo(db *gorm.DB) *AlertRuleRepo {
	return &AlertRuleRepo{db: db}
}

// List returns all rules.
func (r *AlertRuleRepo) List() ([]model.AlertRule, error) {
	var rules []model.AlertRule
	err := r.db.Order("id").Find(&rules).Error
	return rules, err
}

// Create creates a rule.
func (r *AlertRuleRepo) Create(rule *model.AlertRule) error {
	return r.db.Create(rule).Error
}

// Update updates a rule.
func (r *AlertRuleRepo) Update(rule *model.AlertRule) error {
	return r.db.Save(rule).Error
}

// Delete deletes a rule.
func (r *AlertRuleRepo) Delete(id uint) error {
	return r.db.Delete(&model.AlertRule{}, id).Error
}
