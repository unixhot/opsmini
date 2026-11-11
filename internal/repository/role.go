package repository

import (
	"errors"
	"strings"

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

// Seed writes the built-in roles: creates them when absent, and backfills
// permissions that a new version introduced when they already exist.
//
// 「只补不删」是刻意的：管理员可能按自己的规范调过内置角色（比如收紧了
// operator 的终端权限），全量覆盖会把他的调整抹掉。可新版本新增的权限点
// 又必须补上——不然升级后就是「功能在、菜单点得开、接口 403」，每加一个
// 页面就坑一次老用户。所以取并集。
func (r *RoleRepo) Seed() error {
	for _, builtin := range model.BuiltinRoles() {
		var role model.Role
		err := r.db.Where("name = ?", builtin.Name).First(&role).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := r.db.Create(&builtin).Error; err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		// admin 是通配符，已经涵盖一切；不要把它改成一长串具体权限。
		if role.Perms == "*" {
			continue
		}
		merged := mergePerms(role.Perms, builtin.Perms)
		if merged != role.Perms {
			if err := r.db.Model(&model.Role{}).Where("id = ?", role.ID).
				Update("perms", merged).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// mergePerms unions two comma-separated permission lists, keeping the order of
// existing first so a role's own arrangement stays readable.
func mergePerms(existing, add string) string {
	if existing == "" {
		return add
	}
	// 通配符不能凭空造出来：普通角色不会因为新版内置定义写了 "*" 就变成管理员。
	if add == "" || add == "*" {
		return existing
	}
	seen := make(map[string]bool, 16)
	out := make([]string, 0, 32)
	appendAll := func(list string) {
		for _, k := range strings.Split(list, ",") {
			k = strings.TrimSpace(k)
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			out = append(out, k)
		}
	}
	appendAll(existing)
	appendAll(add)
	return strings.Join(out, ",")
}
