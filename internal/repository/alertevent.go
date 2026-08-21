package repository

import (
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// AlertEventRepo provides data access for alert events.
type AlertEventRepo struct {
	db *gorm.DB
}

// NewAlertEventRepo returns a new AlertEventRepo.
func NewAlertEventRepo(db *gorm.DB) *AlertEventRepo {
	return &AlertEventRepo{db: db}
}

// List returns the latest limit events (in reverse chronological order).
func (r *AlertEventRepo) List(limit int) ([]model.AlertEvent, error) {
	if limit <= 0 {
		limit = 200
	}
	out := make([]model.AlertEvent, 0)
	err := r.db.Order("id DESC").Limit(limit).Find(&out).Error
	return out, err
}

// ListActive returns all unresolved (active) events.
func (r *AlertEventRepo) ListActive() ([]model.AlertEvent, error) {
	out := make([]model.AlertEvent, 0)
	err := r.db.Where("status = ?", model.AlertStatusActive).Order("id DESC").Find(&out).Error
	return out, err
}

// CountActive counts unresolved events.
func (r *AlertEventRepo) CountActive() (int64, error) {
	var n int64
	err := r.db.Model(&model.AlertEvent{}).Where("status = ?", model.AlertStatusActive).Count(&n).Error
	return n, err
}

// FindActiveByRule finds the current active event of a rule.
func (r *AlertEventRepo) FindActiveByRule(ruleID uint) (*model.AlertEvent, error) {
	var e model.AlertEvent
	err := r.db.Where("rule_id = ? AND status = ?", ruleID, model.AlertStatusActive).First(&e).Error
	if err != nil {
		return nil, err
	}
	return &e, nil
}

// Create creates an event.
func (r *AlertEventRepo) Create(e *model.AlertEvent) error {
	return r.db.Create(e).Error
}

// Update updates an event.
func (r *AlertEventRepo) Update(e *model.AlertEvent) error {
	return r.db.Save(e).Error
}

// Delete deletes an event.
func (r *AlertEventRepo) Delete(id uint) error {
	return r.db.Delete(&model.AlertEvent{}, id).Error
}

// DeleteOlderThan deletes events older than cutoff (for retention cleanup).
func (r *AlertEventRepo) DeleteOlderThan(cutoff time.Time) (int64, error) {
	res := r.db.Where("created_at < ?", cutoff).Delete(&model.AlertEvent{})
	return res.RowsAffected, res.Error
}
