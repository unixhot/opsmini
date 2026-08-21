package repository

import (
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// AccessLogRepo provides data access for panel access logs.
type AccessLogRepo struct {
	db *gorm.DB
}

// NewAccessLogRepo returns a new AccessLogRepo.
func NewAccessLogRepo(db *gorm.DB) *AccessLogRepo {
	return &AccessLogRepo{db: db}
}

// Create records one access log entry.
func (r *AccessLogRepo) Create(e *model.AccessLog) error {
	return r.db.Create(e).Error
}

// List returns the latest limit access log entries (in reverse chronological order).
func (r *AccessLogRepo) List(limit int) ([]model.AccessLog, error) {
	if limit <= 0 {
		limit = 200
	}
	out := make([]model.AccessLog, 0)
	err := r.db.Order("id DESC").Limit(limit).Find(&out).Error
	return out, err
}

// DeleteOlderThan deletes access logs older than cutoff.
func (r *AccessLogRepo) DeleteOlderThan(cutoff time.Time) (int64, error) {
	res := r.db.Where("created_at < ?", cutoff).Delete(&model.AccessLog{})
	return res.RowsAffected, res.Error
}
