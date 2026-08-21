package repository

import (
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// RoleRepo provides data access for roles.
type RoleRepo struct {
	db *gorm.DB
}

// NewRoleRepo returns a new RoleRepo.
func NewRoleRepo(db *gorm.DB) *RoleRepo {
	return &RoleRepo{db: db}
}

// List returns all roles.
func (r *RoleRepo) List() ([]model.Role, error) {
	out := make([]model.Role, 0)
	err := r.db.Order("id").Find(&out).Error
	return out, err
}

// FindByName finds a role by name.
func (r *RoleRepo) FindByName(name string) (*model.Role, error) {
	var role model.Role
	if err := r.db.Where("name = ?", name).First(&role).Error; err != nil {
		return nil, err
	}
	return &role, nil
}

// Create creates a role.
func (r *RoleRepo) Create(role *model.Role) error {
	return r.db.Create(role).Error
}

// Update updates a role.
func (r *RoleRepo) Update(role *model.Role) error {
	return r.db.Save(role).Error
}

// Delete deletes a role.
func (r *RoleRepo) Delete(id uint) error {
	return r.db.Delete(&model.Role{}, id).Error
}

// Seed writes the built-in roles (creates them if absent; does not overwrite existing custom modifications).
func (r *RoleRepo) Seed() error {
	for _, builtin := range model.BuiltinRoles() {
		var count int64
		if err := r.db.Model(&model.Role{}).Where("name = ?", builtin.Name).Count(&count).Error; err != nil {
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
