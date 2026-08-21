package repository

import (
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// AuditLogRepo provides data access for audit logs.
type AuditLogRepo struct {
	db *gorm.DB
}

// NewAuditLogRepo returns a new AuditLogRepo.
func NewAuditLogRepo(db *gorm.DB) *AuditLogRepo {
	return &AuditLogRepo{db: db}
}

// Create writes one audit log entry.
func (r *AuditLogRepo) Create(l *model.AuditLog) error { return r.db.Create(l).Error }

// List returns the latest limit entries in reverse chronological order.
func (r *AuditLogRepo) List(limit int) ([]model.AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var list []model.AuditLog
	err := r.db.Order("id desc").Limit(limit).Find(&list).Error
	return list, err
}

// DeleteOlderThan deletes audit logs older than cutoff (for retention cleanup).
func (r *AuditLogRepo) DeleteOlderThan(cutoff time.Time) (int64, error) {
	res := r.db.Where("created_at < ?", cutoff).Delete(&model.AuditLog{})
	return res.RowsAffected, res.Error
}
