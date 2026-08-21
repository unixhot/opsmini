package repository

import (
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// BaselineResultRepo provides data access for baseline check results.
type BaselineResultRepo struct {
	db *gorm.DB
}

// NewBaselineResultRepo returns a new BaselineResultRepo.
func NewBaselineResultRepo(db *gorm.DB) *BaselineResultRepo {
	return &BaselineResultRepo{db: db}
}

// Create writes one check result.
func (r *BaselineResultRepo) Create(b *model.BaselineResult) error {
	return r.db.Create(b).Error
}

// ListByScanID lists check results by batch number.
func (r *BaselineResultRepo) ListByScanID(scanID string) ([]model.BaselineResult, error) {
	out := make([]model.BaselineResult, 0)
	err := r.db.Where("scan_id = ?", scanID).Order("id").Find(&out).Error
	return out, err
}

// LatestScanID returns the batch number of the latest scan.
func (r *BaselineResultRepo) LatestScanID() (string, error) {
	var row model.BaselineResult
	err := r.db.Order("id DESC").First(&row).Error
	if err != nil {
		return "", err
	}
	return row.ScanID, nil
}

// DeleteOlderThan deletes results older than cutoff.
func (r *BaselineResultRepo) DeleteOlderThan(cutoff time.Time) (int64, error) {
	res := r.db.Where("created_at < ?", cutoff).Delete(&model.BaselineResult{})
	return res.RowsAffected, res.Error
}
