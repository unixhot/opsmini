// Package repository encapsulates the data access layer.
package repository

import (
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// AppRepo provides data access for software store apps.
type AppRepo struct {
	db *gorm.DB
}

// NewAppRepo returns a new AppRepo.
func NewAppRepo(db *gorm.DB) *AppRepo {
	return &AppRepo{db: db}
}

// List returns all apps.
func (r *AppRepo) List() ([]model.App, error) {
	var apps []model.App
	err := r.db.Order("id").Find(&apps).Error
	return apps, err
}

// FindBySlug finds an app by slug.
func (r *AppRepo) FindBySlug(slug string) (*model.App, error) {
	var a model.App
	if err := r.db.Where("slug = ?", slug).First(&a).Error; err != nil {
		return nil, err
	}
	return &a, nil
}

// Upsert inserts or updates an app (for seed data).
func (r *AppRepo) Upsert(a *model.App) error {
	return r.db.Save(a).Error
}

// DeleteBySlug deletes an app template by slug.
func (r *AppRepo) DeleteBySlug(slug string) error {
	return r.db.Where("slug = ?", slug).Delete(&model.App{}).Error
}
