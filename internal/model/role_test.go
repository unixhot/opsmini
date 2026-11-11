package model

import (
	"strings"
	"testing"
)

func hasPermIn(list, key string) bool {
	for _, k := range strings.Split(list, ",") {
		if strings.TrimSpace(k) == key {
			return true
		}
	}
	return false
}

// 只读角色必须能"看"，但不能"处置"。
//
// 事件中心原来借用 ai.agent：只读角色收集不到这个权限点，于是菜单点得开、
// 进去一片 403——用户以为面板坏了。同一个人却能点批准、接管、回滚，
// 因为查看和处置共用同一个权限点。这两件事必须分开。
func TestReadonlyCanViewButNotAct(t *testing.T) {
	perms := readonlyPerms()
	if !hasPermIn(perms, "sentinel.view") {
		t.Fatalf("只读角色应能查看事件中心，实际权限: %s", perms)
	}
	if hasPermIn(perms, "sentinel.act") {
		t.Fatal("只读角色不该能处置事件")
	}
	// 只读角色只收 .view，别的写权限也不能漏进来。
	for _, k := range strings.Split(perms, ",") {
		if k == "" {
			continue
		}
		if !strings.HasSuffix(k, ".view") {
			t.Fatalf("只读角色混入了非查看权限: %s", k)
		}
	}
	// 变更记录也属于"看"，只读用户同样需要。之前它挂在 ai.agent 上，
	// 只读用户点「变更记录」同样是 403。
	if !hasPermIn(perms, "sentinel.view") {
		t.Fatal("只读角色应能查看变更记录")
	}
}

// 运维角色要能处置事件，否则出事时没人能批准。
func TestOperatorCanAct(t *testing.T) {
	perms := operatorPerms()
	for _, k := range []string{"sentinel.view", "sentinel.act", "ai.agent"} {
		if !hasPermIn(perms, k) {
			t.Fatalf("运维角色缺少 %s", k)
		}
	}
	// 但改设置和管用户仍然不行。
	for _, k := range []string{"settings.edit", "user.create"} {
		if hasPermIn(perms, k) {
			t.Fatalf("运维角色不该有 %s", k)
		}
	}
}

// 管理员是通配符，新权限点自动生效，不需要改任何种子数据。
func TestAdminIsWildcard(t *testing.T) {
	for _, r := range BuiltinRoles() {
		if r.Name == RoleAdmin && r.Perms != "*" {
			t.Fatalf("管理员应为通配符，实际 %q", r.Perms)
		}
	}
}
