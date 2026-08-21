package repository

import (
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// AppCategoryRepo provides data access for app categories.
type AppCategoryRepo struct {
	db *gorm.DB
}

// NewAppCategoryRepo returns a new AppCategoryRepo.
func NewAppCategoryRepo(db *gorm.DB) *AppCategoryRepo {
	return &AppCategoryRepo{db: db}
}

// List returns all categories.
func (r *AppCategoryRepo) List() ([]model.AppCategory, error) {
	out := make([]model.AppCategory, 0)
	err := r.db.Order("id").Find(&out).Error
	return out, err
}

// FindByKey finds a category by Key.
func (r *AppCategoryRepo) FindByKey(key string) (*model.AppCategory, error) {
	var c model.AppCategory
	if err := r.db.Where("key = ?", key).First(&c).Error; err != nil {
		return nil, err
	}
	return &c, nil
}

// Create creates a category.
func (r *AppCategoryRepo) Create(c *model.AppCategory) error {
	return r.db.Create(c).Error
}

// Update updates a category.
func (r *AppCategoryRepo) Update(c *model.AppCategory) error {
	return r.db.Save(c).Error
}

// Delete deletes a category.
func (r *AppCategoryRepo) Delete(id uint) error {
	return r.db.Delete(&model.AppCategory{}, id).Error
}

// Seed writes the four built-in categories (creates them if absent).
func (r *AppCategoryRepo) Seed() error {
	for _, builtin := range model.BuiltinCategories() {
		var count int64
		if err := r.db.Model(&model.AppCategory{}).Where("key = ?", builtin.Key).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := r.db.Create(&builtin).Error; err != nil {
				return err
			}
		}
	}
	return nil
}
