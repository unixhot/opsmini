package repository

import (
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// DatabaseRepo provides data access for databases.
type DatabaseRepo struct {
	db *gorm.DB
}

// NewDatabaseRepo returns a new DatabaseRepo.
func NewDatabaseRepo(db *gorm.DB) *DatabaseRepo {
	return &DatabaseRepo{db: db}
}

// Create creates a database instance.
func (r *DatabaseRepo) Create(d *model.Database) error { return r.db.Create(d).Error }

// Update saves a database instance.
func (r *DatabaseRepo) Update(d *model.Database) error { return r.db.Save(d).Error }

// Delete deletes a database instance.
func (r *DatabaseRepo) Delete(id uint) error { return r.db.Delete(&model.Database{}, id).Error }

// FindByID finds a database by ID.
func (r *DatabaseRepo) FindByID(id uint) (*model.Database, error) {
	var d model.Database
	if err := r.db.First(&d, id).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

// List returns all database instances.
func (r *DatabaseRepo) List() ([]model.Database, error) {
	var list []model.Database
	err := r.db.Order("id").Find(&list).Error
	return list, err
}
