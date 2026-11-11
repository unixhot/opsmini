package store

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opsmini/opsmini/internal/model"
)

// 启动时跑的那个种子函数必须会把新增权限点补到内置角色上。
//
// 这条测试的价值在于：它测的是 store.seedRoles——真正被 Init 调用的那个。
// 之前这个函数是仓库层 Seed() 的一份复制品（只在角色不存在时创建），
// 于是任何"给内置角色补权限"的改动都只改了没人调用的那一份，升级上来的库
// 永远拿不到新权限点。测对了函数，才守得住行为。
func TestSeedRolesBackfillsOnUpgrade(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Role{}); err != nil {
		t.Fatal(err)
	}

	// 模拟从 v1.4 升级上来的库：只读角色只有旧版那几项。
	if err := db.Create(&model.Role{
		Name: model.RoleReadonly, Label: "只读", Builtin: true,
		Perms: "security.view,monitor.view",
	}).Error; err != nil {
		t.Fatal(err)
	}

	if err := seedRoles(db); err != nil {
		t.Fatalf("seedRoles: %v", err)
	}

	var ro model.Role
	if err := db.Where("name = ?", model.RoleReadonly).First(&ro).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ro.Perms, "sentinel.view") {
		t.Fatalf("升级后只读角色应补上 sentinel.view，实际: %s", ro.Perms)
	}
	if strings.Contains(ro.Perms, "sentinel.act") {
		t.Fatalf("只读角色不该有 sentinel.act: %s", ro.Perms)
	}
	if !strings.Contains(ro.Perms, "monitor.view") {
		t.Fatalf("原有权限被删: %s", ro.Perms)
	}
}
