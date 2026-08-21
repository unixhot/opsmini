package repository

import (
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// FimBaselineRepo provides data access for file integrity baselines.
type FimBaselineRepo struct {
	db *gorm.DB
}

// NewFimBaselineRepo returns a new FimBaselineRepo.
func NewFimBaselineRepo(db *gorm.DB) *FimBaselineRepo {
	return &FimBaselineRepo{db: db}
}

// List returns all baseline items.
func (r *FimBaselineRepo) List() ([]model.FimBaseline, error) {
	out := make([]model.FimBaseline, 0)
	err := r.db.Order("id").Find(&out).Error
	return out, err
}

// Count returns the number of baseline items.
func (r *FimBaselineRepo) Count() (int64, error) {
	var n int64
	err := r.db.Model(&model.FimBaseline{}).Count(&n).Error
	return n, err
}

// Upsert inserts or updates a baseline by path (including the latest hash).
func (r *FimBaselineRepo) Upsert(b *model.FimBaseline) error {
	var existing model.FimBaseline
	err := r.db.Where("path = ?", b.Path).First(&existing).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return r.db.Create(b).Error
		}
		return err
	}
	existing.SHA256 = b.SHA256
	existing.Size = b.Size
	existing.ModTime = b.ModTime
	return r.db.Save(&existing).Error
}

// Delete deletes a baseline item by ID.
func (r *FimBaselineRepo) Delete(id uint) error {
	return r.db.Delete(&model.FimBaseline{}, id).Error
}

// FimChangeRepo provides data access for file integrity change events.
type FimChangeRepo struct {
	db *gorm.DB
}

// NewFimChangeRepo returns a new FimChangeRepo.
func NewFimChangeRepo(db *gorm.DB) *FimChangeRepo {
	return &FimChangeRepo{db: db}
}

// Create writes one change event.
func (r *FimChangeRepo) Create(c *model.FimChange) error {
	return r.db.Create(c).Error
}

// List returns the latest limit change events.
func (r *FimChangeRepo) List(limit int) ([]model.FimChange, error) {
	if limit <= 0 {
		limit = 200
	}
	out := make([]model.FimChange, 0)
	err := r.db.Order("id DESC").Limit(limit).Find(&out).Error
	return out, err
}

// CountRecent counts change events within the recent duration.
func (r *FimChangeRepo) CountRecent(since time.Time) (int64, error) {
	var n int64
	err := r.db.Model(&model.FimChange{}).Where("created_at >= ?", since).Count(&n).Error
	return n, err
}
