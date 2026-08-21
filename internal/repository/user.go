// Package repository encapsulates the data access layer.
package repository

import (
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// UserRepo provides data access for users.
type UserRepo struct {
	db *gorm.DB
}

// NewUserRepo returns a new UserRepo.
func NewUserRepo(db *gorm.DB) *UserRepo {
	return &UserRepo{db: db}
}

// Create creates a user.
func (r *UserRepo) Create(u *model.User) error {
	return r.db.Create(u).Error
}

// FindByUsername finds a user by username.
func (r *UserRepo) FindByUsername(username string) (*model.User, error) {
	var u model.User
	if err := r.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// FindByID finds a user by ID.
func (r *UserRepo) FindByID(id uint) (*model.User, error) {
	var u model.User
	if err := r.db.First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// List returns all users.
func (r *UserRepo) List() ([]model.User, error) {
	var users []model.User
	err := r.db.Order("id").Find(&users).Error
	return users, err
}

// Update saves a user.
func (r *UserRepo) Update(u *model.User) error {
	return r.db.Save(u).Error
}

// Delete deletes a user by ID.
func (r *UserRepo) Delete(id uint) error {
	return r.db.Delete(&model.User{}, id).Error
}
