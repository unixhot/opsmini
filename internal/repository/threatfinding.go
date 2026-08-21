package repository

import (
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// ThreatFindingRepo provides data access for threat findings.
type ThreatFindingRepo struct {
	db *gorm.DB
}

// NewThreatFindingRepo returns a new ThreatFindingRepo.
func NewThreatFindingRepo(db *gorm.DB) *ThreatFindingRepo {
	return &ThreatFindingRepo{db: db}
}

// Create writes one threat finding.
func (r *ThreatFindingRepo) Create(t *model.ThreatFinding) error {
	return r.db.Create(t).Error
}

// List returns the latest limit threats.
func (r *ThreatFindingRepo) List(limit int) ([]model.ThreatFinding, error) {
	if limit <= 0 {
		limit = 200
	}
	out := make([]model.ThreatFinding, 0)
	err := r.db.Order("id DESC").Limit(limit).Find(&out).Error
	return out, err
}

// ListActive returns all unresolved (active) threats.
func (r *ThreatFindingRepo) ListActive() ([]model.ThreatFinding, error) {
	out := make([]model.ThreatFinding, 0)
	err := r.db.Where("status = ?", "active").Order("id DESC").Find(&out).Error
	return out, err
}

// CountActive counts unresolved threats.
func (r *ThreatFindingRepo) CountActive() (int64, error) {
	var n int64
	err := r.db.Model(&model.ThreatFinding{}).Where("status = ?", "active").Count(&n).Error
	return n, err
}

// FindByKindName finds a threat by kind + name (for deduplication).
func (r *ThreatFindingRepo) FindByKindName(kind, name string) (*model.ThreatFinding, error) {
	var t model.ThreatFinding
	err := r.db.Where("kind = ? AND name = ?", kind, name).First(&t).Error
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// Update updates a threat.
func (r *ThreatFindingRepo) Update(t *model.ThreatFinding) error {
	return r.db.Save(t).Error
}

// DeleteOlderThan deletes resolved threats older than cutoff.
func (r *ThreatFindingRepo) DeleteOlderThan(cutoff time.Time) (int64, error) {
	res := r.db.Where("status = ? AND resolved_at < ?", "resolved", cutoff).Delete(&model.ThreatFinding{})
	return res.RowsAffected, res.Error
}
