//go:build windows

package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/sys/windows"
)

// Windows 防火墙后端。
//
// 读和写走两条路：
//   - 读：PowerShell Get-NetFirewallRule。netsh 的输出字段名会被本地化
//     （中文系统上是「规则名称 / 操作 / 本地端口」），解析必然出错，而
//     PowerShell 返回的是对象属性，名字不翻译。
//   - 写：netsh advfirewall。增删改各一条命令，比每次 CIM 往返快得多，也不受
//     执行策略影响（netsh 是原生 exe）。

// windowsRulePrefix 标记面板创建的规则，方便列表里优先展示与区分。
const windowsRulePrefix = "OpsMini-"

// isPrivileged reports whether the process runs with an elevated token.
// `os.Geteuid()` returns -1 on Windows, so the Unix check rejected everything.
func isPrivileged() bool {
	tok := windows.GetCurrentProcessToken()
	return tok.IsElevated()
}

// remoteAdminPort is the port kept open when enabling the firewall, so the
// operator cannot lock themselves out: RDP on Windows, SSH elsewhere.
func remoteAdminPort() int { return 3389 }

// windowsFirewallRules lists inbound rules, panel-created ones first.
func windowsFirewallRules() []FirewallRule {
	type row struct {
		Name  string `json:"Name"`
		Proto string `json:"Proto"`
		Port  string `json:"Port"`
	}

	// 只取面板创建的规则 + 前 60 条已启用入站规则：一台桌面机上入站规则有几百条，
	// 全量返回既慢又没人看得过来。
	const q = `Get-NetFirewallRule -Direction Inbound -Enabled True | Select-Object -First 60 | ForEach-Object { $p = $_ | Get-NetFirewallPortFilter; [pscustomobject]@{ Name=$_.DisplayName; Proto=[string]$p.Protocol; Port=[string]($p.LocalPort -join ',') } }`
	var rows []row
	if err := psJSON(q, &rows); err != nil {
		return nil
	}

	out := make([]FirewallRule, 0, len(rows))
	for _, r := range rows {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}
		action := "allow"
		if strings.Contains(strings.ToLower(name), "block") || strings.Contains(name, "deny") {
			action = "deny"
		}
		out = append(out, FirewallRule{
			Ref:    name,
			Proto:  strings.ToLower(r.Proto),
			Port:   r.Port,
			Action: action,
			Raw:    fmt.Sprintf("%s %s/%s", name, strings.ToLower(r.Proto), r.Port),
		})
	}
	return out
}

// windowsAllowPort creates (or refreshes) an inbound allow rule.
func windowsAllowPort(port int, proto string) error {
	return windowsWriteRule("allow", port, proto)
}

// windowsDenyPort creates (or refreshes) an inbound block rule.
func windowsDenyPort(port int, proto string) error {
	return windowsWriteRule("block", port, proto)
}

// windowsWriteRule 用固定规则名覆盖式写入：先删同名再加，避免同名规则越积越多。
func windowsWriteRule(action string, port int, proto string) error {
	name := fmt.Sprintf("%s%s %d/%s", windowsRulePrefix, action, port, proto)
	_, _ = runFirewallCmd("netsh", "advfirewall", "firewall", "delete", "rule", "name="+name)
	_, err := runFirewallCmd("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+name, "dir=in", "action="+action,
		"protocol="+strings.ToUpper(proto),
		"localport="+strconv.Itoa(port),
		"profile=any", "enable=yes")
	return err
}

// windowsDeleteRule removes a rule by its display name (Ref from the list).
func windowsDeleteRule(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return errors.New("规则名不能为空")
	}
	_, err := runFirewallCmd("netsh", "advfirewall", "firewall", "delete", "rule", "name="+ref)
	return err
}

// windowsEnableFirewall turns the firewall on for every profile.
func windowsEnableFirewall() error {
	_, err := runFirewallCmd("netsh", "advfirewall", "set", "allprofiles", "state", "on")
	return err
}

// windowsDisableFirewall turns the firewall off for every profile.
func windowsDisableFirewall() error {
	_, err := runFirewallCmd("netsh", "advfirewall", "set", "allprofiles", "state", "off")
	return err
}
