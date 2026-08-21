package repository

import (
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// WebsiteRepo provides data access for websites.
type WebsiteRepo struct {
	db *gorm.DB
}

// NewWebsiteRepo returns a new WebsiteRepo.
func NewWebsiteRepo(db *gorm.DB) *WebsiteRepo {
	return &WebsiteRepo{db: db}
}

// Create creates a website.
func (r *WebsiteRepo) Create(w *model.Website) error { return r.db.Create(w).Error }

// Update saves a website.
func (r *WebsiteRepo) Update(w *model.Website) error { return r.db.Save(w).Error }

// Delete deletes a website.
func (r *WebsiteRepo) Delete(id uint) error { return r.db.Delete(&model.Website{}, id).Error }

// FindByID finds a website by ID.
func (r *WebsiteRepo) FindByID(id uint) (*model.Website, error) {
	var w model.Website
	if err := r.db.First(&w, id).Error; err != nil {
		return nil, err
	}
	return &w, nil
}

// List returns all websites.
func (r *WebsiteRepo) List() ([]model.Website, error) {
	var list []model.Website
	err := r.db.Order("id").Find(&list).Error
	return list, err
}
