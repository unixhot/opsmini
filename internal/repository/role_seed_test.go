package repository

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opsmini/opsmini/internal/model"
)

func newRoleDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Role{}); err != nil {
		t.Fatal(err)
	}
	return db
}

// 老库升级后，内置角色必须拿到新版本新增的权限点。
//
// Seed 原来只在角色不存在时创建，已存在的一概不碰。于是每次给内置角色加
// 权限（比如这次的 sentinel.view），升级上来的用户就是「功能在、菜单点得开、
// 接口全部 403」——每次发版都坑一次。
func TestSeedBackfillsNewPermissions(t *testing.T) {
	db := newRoleDB(t)
	repo := NewRoleRepo(db)

	// 模拟老库：只读角色只认识旧版本的几个权限点。
	old := model.Role{Name: model.RoleReadonly, Label: "只读", Builtin: true,
		Perms: "security.view,monitor.view"}
	if err := db.Create(&old).Error; err != nil {
		t.Fatal(err)
	}

	if err := repo.Seed(); err != nil {
		t.Fatal(err)
	}

	got, err := repo.FindByName(model.RoleReadonly)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Perms, "sentinel.view") {
		t.Fatalf("升级后只读角色应补上 sentinel.view，实际: %s", got.Perms)
	}
	if strings.Contains(got.Perms, "sentinel.act") {
		t.Fatalf("只读角色不该被补上 sentinel.act: %s", got.Perms)
	}
	// 原有的权限不能被删。
	for _, k := range []string{"security.view", "monitor.view"} {
		if !strings.Contains(got.Perms, k) {
			t.Fatalf("原有权限 %s 被删掉了: %s", k, got.Perms)
		}
	}
	// 内置角色名下的其他字段不该被动。
	if got.Label != "只读" {
		t.Fatalf("label 被改动: %q", got.Label)
	}
}

// 管理员手动给内置角色加的自定义权限不能被 Seed 抹掉。
func TestSeedKeepsManualGrants(t *testing.T) {
	db := newRoleDB(t)
	repo := NewRoleRepo(db)

	custom := model.Role{Name: model.RoleReadonly, Label: "只读", Builtin: true,
		Perms: "security.view,custom.special"}
	if err := db.Create(&custom).Error; err != nil {
		t.Fatal(err)
	}
	if err := repo.Seed(); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.FindByName(model.RoleReadonly)
	if !strings.Contains(got.Perms, "custom.special") {
		t.Fatalf("管理员手动加的权限被删了: %s", got.Perms)
	}
}

// 管理员是通配符，Seed 不能把它改成一长串具体权限。
func TestSeedLeavesAdminWildcard(t *testing.T) {
	db := newRoleDB(t)
	repo := NewRoleRepo(db)
	if err := repo.Seed(); err != nil {
		t.Fatal(err)
	}
	admin, err := repo.FindByName(model.RoleAdmin)
	if err != nil {
		t.Fatal(err)
	}
	if admin.Perms != "*" {
		t.Fatalf("管理员权限应为 *，实际 %q", admin.Perms)
	}

	// 重复执行要幂等。
	if err := repo.Seed(); err != nil {
		t.Fatal(err)
	}
	var n int64
	db.Model(&model.Role{}).Count(&n)
	if n != 3 {
		t.Fatalf("重复 Seed 后角色数应为 3，实际 %d", n)
	}
}

func TestMergePerms(t *testing.T) {
	cases := []struct{ a, b, want string }{
		{"", "x,y", "x,y"},
		{"x,y", "", "x,y"},
		{"x", "x,y", "x,y"},
		{"y,x", "x,z", "y,x,z"},
		{"a,a,b", "b,c", "a,b,c"},
		// 普通角色不会因为内置定义写了 "*" 就变成管理员。
		{"a", "*", "a"},
	}
	for _, c := range cases {
		if got := mergePerms(c.a, c.b); got != c.want {
			t.Errorf("mergePerms(%q,%q)=%q，期望 %q", c.a, c.b, got, c.want)
		}
	}
}
