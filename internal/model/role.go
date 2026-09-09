// Package model defines GORM data models and the permission registry.
package model

import "time"

// Role is a panel role (built-in + custom). Name is the unique identifier; user.role stores the role Name.
type Role struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Name      string    `gorm:"size:64;uniqueIndex" json:"name"` // admin/operator/readonly/custom name
	Label     string    `gorm:"size:64" json:"label"`            // display name, e.g. administrator/operator/read-only
	Perms     string    `gorm:"type:text" json:"perms"`          // comma-separated permission keys; admin is always "*"
	Builtin   bool      `gorm:"default:false" json:"builtin"`    // whether it is a built-in role
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Permission is a single permission point.
type Permission struct {
	Key   string `json:"key"`   // permission key, e.g. website.create
	Label string `json:"label"` // description, e.g. create website
}

// PermissionGroup is a permission group (for display on the frontend role management page).
type PermissionGroup struct {
	Key   string       `json:"key"`
	Label string       `json:"label"`
	Perms []Permission `json:"perms"`
}

// perm builds a Permission from a key and a display label.
func perm(key, label string) Permission { return Permission{Key: key, Label: label} }

// PermGroups returns all permission groups (hard-coded, the single source of truth).
func PermGroups() []PermissionGroup {
	return []PermissionGroup{
		{Key: "user", Label: "用户管理", Perms: []Permission{
			perm("user.view", "查看用户"), perm("user.create", "创建用户"),
			perm("user.edit", "编辑用户"), perm("user.delete", "删除用户"),
		}},
		{Key: "website", Label: "网站管理", Perms: []Permission{
			perm("website.view", "查看网站"), perm("website.create", "创建网站"),
			perm("website.edit", "编辑网站"), perm("website.delete", "删除网站"),
		}},
		{Key: "database", Label: "数据库", Perms: []Permission{
			perm("database.view", "查看数据库"), perm("database.create", "创建数据库"),
			perm("database.edit", "编辑数据库"), perm("database.delete", "删除数据库"),
		}},
		{Key: "cron", Label: "计划任务", Perms: []Permission{
			perm("cron.view", "查看计划任务"), perm("cron.create", "创建计划任务"),
			perm("cron.edit", "编辑计划任务"), perm("cron.delete", "删除计划任务"),
		}},
		{Key: "file", Label: "文件管理", Perms: []Permission{
			perm("file.view", "查看文件"), perm("file.write", "上传/新建"),
			perm("file.delete", "删除文件"),
		}},
		{Key: "container", Label: "容器管理", Perms: []Permission{
			perm("container.view", "查看容器"), perm("container.create", "创建容器"),
			perm("container.edit", "启停容器"), perm("container.delete", "删除容器"),
		}},
		{Key: "apps", Label: "应用商店", Perms: []Permission{
			perm("apps.view", "查看应用"), perm("apps.install", "安装/卸载应用"),
		}},
		{Key: "monitor", Label: "监控", Perms: []Permission{
			perm("monitor.view", "查看监控"),
		}},
		{Key: "system", Label: "系统管理", Perms: []Permission{
			perm("system.view", "查看系统信息"), perm("system.process", "进程管理"),
			perm("system.firewall", "防火墙管理"),
		}},
		{Key: "security", Label: "主机安全", Perms: []Permission{
			perm("security.view", "查看安全概览"), perm("security.scan", "基线体检"),
			perm("security.firewall", "防火墙规则"), perm("security.fim", "文件完整性"),
			perm("security.threat", "威胁处理"),
		}},
		{Key: "alert", Label: "告警", Perms: []Permission{
			perm("alert.view", "查看告警"), perm("alert.manage", "管理告警规则"),
		}},
		{Key: "ai", Label: "AI 助手", Perms: []Permission{
			perm("ai.use", "使用 AI 助手"),
		}},
		{Key: "terminal", Label: "终端", Perms: []Permission{
			perm("terminal.use", "使用 Web 终端"),
		}},
		{Key: "mcp", Label: "MCP 服务", Perms: []Permission{
			perm("mcp.view", "查看 MCP"), perm("mcp.manage", "管理 MCP"),
		}},
		{Key: "skill", Label: "技能（Skill）", Perms: []Permission{
			perm("skill.view", "查看技能"), perm("skill.manage", "安装/管理技能"),
		}},
		{Key: "log", Label: "日志", Perms: []Permission{
			perm("log.view", "查看系统日志"), perm("audit.view", "查看审计日志"),
		}},
		{Key: "settings", Label: "面板设置", Perms: []Permission{
			perm("settings.view", "查看设置"), perm("settings.edit", "修改设置"),
		}},
	}
}

// AllPermKeys returns all permission keys (used to expand admin's full permissions).
func AllPermKeys() []string {
	var keys []string
	for _, g := range PermGroups() {
		for _, p := range g.Perms {
			keys = append(keys, p.Key)
		}
	}
	return keys
}

// BuiltinRoles returns the three built-in roles and their default permissions.
func BuiltinRoles() []Role {
	return []Role{
		{Name: RoleAdmin, Label: "管理员", Perms: "*", Builtin: true},
		{Name: RoleOperator, Label: "运维", Builtin: true, Perms: operatorPerms()},
		{Name: RoleReadonly, Label: "只读", Builtin: true, Perms: readonlyPerms()},
	}
}

// operatorPerms returns default operator role permissions: all operations except user management and panel settings, plus all view permissions.
func operatorPerms() string {
	exclude := map[string]bool{
		"user.create": true, "user.edit": true, "user.delete": true,
		"settings.edit": true,
	}
	var keys []string
	for _, k := range AllPermKeys() {
		if !exclude[k] {
			keys = append(keys, k)
		}
	}
	return joinPerms(keys)
}

// readonlyPerms returns default read-only role permissions: only all view permissions.
func readonlyPerms() string {
	var keys []string
	for _, k := range AllPermKeys() {
		if len(k) > 5 && k[len(k)-5:] == ".view" {
			keys = append(keys, k)
		}
	}
	return joinPerms(keys)
}

// joinPerms joins permission keys into a single comma-separated string.
func joinPerms(keys []string) string {
	out := ""
	for i, k := range keys {
		if i > 0 {
			out += ","
		}
		out += k
	}
	return out
}
