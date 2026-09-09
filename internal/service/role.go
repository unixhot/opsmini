// Package service is the business layer.
package service

import (
	"errors"
	"strings"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// RoleService handles roles and permissions.
type RoleService struct {
	repo *repository.RoleRepo
}

// NewRoleService creates a RoleService.
func NewRoleService(repo *repository.RoleRepo) *RoleService {
	return &RoleService{repo: repo}
}

// List returns all roles.
func (s *RoleService) List() ([]model.Role, error) {
	return s.repo.List()
}

// Groups returns all permission groups (shown on the frontend role management page).
func (s *RoleService) Groups() []model.PermissionGroup {
	return model.PermGroups()
}

// HasPerm reports whether a role has the given permission. admin has all permissions.
func (s *RoleService) HasPerm(roleName, perm string) bool {
	if roleName == "" || perm == "" {
		return false
	}
	if roleName == model.RoleAdmin {
		return true
	}
	role, err := s.repo.FindByName(roleName)
	if err != nil {
		return false
	}
	return hasPerm(role.Perms, perm)
}

// PermsOf returns the list of permissions a role holds (admin returns all).
func (s *RoleService) PermsOf(roleName string) []string {
	if roleName == model.RoleAdmin {
		return model.AllPermKeys()
	}
	role, err := s.repo.FindByName(roleName)
	if err != nil {
		return []string{}
	}
	var out []string
	for _, p := range strings.Split(role.Perms, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// hasPerm reports whether the comma-separated perms string contains the given permission.
func hasPerm(perms, perm string) bool {
	for _, p := range strings.Split(perms, ",") {
		if strings.TrimSpace(p) == perm {
			return true
		}
	}
	return false
}

// Create creates a custom role.
func (s *RoleService) Create(name, label, perms string) (*model.Role, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("角色标识不能为空")
	}
	if _, err := s.repo.FindByName(name); err == nil {
		return nil, errors.New("角色标识已存在")
	}
	role := &model.Role{Name: name, Label: label, Perms: perms, Builtin: false}
	if err := s.repo.Create(role); err != nil {
		return nil, err
	}
	return role, nil
}

// Update updates a role (built-in roles can only adjust permissions, not the identifier).
func (s *RoleService) Update(id uint, name, label, perms string) error {
	roles, err := s.repo.List()
	if err != nil {
		return err
	}
	var target *model.Role
	for i := range roles {
		if roles[i].ID == id {
			target = &roles[i]
			break
		}
	}
	if target == nil {
		return errors.New("角色不存在")
	}
	if target.Builtin {
		// built-in roles keep their identifier and name; only update permissions
		target.Perms = perms
	} else {
		target.Name = name
		target.Label = label
		target.Perms = perms
	}
	return s.repo.Update(target)
}

// Delete deletes a custom role (built-in roles cannot be deleted).
func (s *RoleService) Delete(id uint) error {
	roles, err := s.repo.List()
	if err != nil {
		return err
	}
	for _, r := range roles {
		if r.ID == id {
			if r.Builtin {
				return errors.New("内置角色不可删除")
			}
			return s.repo.Delete(id)
		}
	}
	return errors.New("角色不存在")
}
