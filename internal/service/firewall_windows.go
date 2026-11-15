//go:build windows

package service

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

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

// firewallQueryTimeout 给列举规则用 30 秒，而非常规的 10 秒。
//
// 实测（172 条入站规则的桌面机）：Get-NetFirewallRule 取 60 条约 2.9 s，再对每条
// 调一次 Get-NetFirewallPortFilter 约 4.2 s，加上进程启动与 JSON 序列化正好在
// 10 s 上下 —— 也就是说默认超时会把一个"慢"的查询变成"完全拿不到数据"。
// 批量取法不可行：非提权进程调用不带管道绑定的 Get-NetFirewallPortFilter 会被
// 拒绝访问，逐条取是唯一可行路径。
const firewallQueryTimeout = 30 * time.Second

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
func windowsFirewallRules() ([]FirewallRule, error) {
	type row struct {
		Name   string `json:"Name"`
		Proto  string `json:"Proto"`
		Port   string `json:"Port"`
		Action string `json:"Action"`
	}

	// 一台桌面机上入站规则有几百条，全量返回既慢又没人看得过来，所以截到 60 条。
	// 截断必须按「面板规则优先」来排：OpsMini- 开头的规则多半不在默认顺序的前 60
	// 条里，用户刚加的规则看不到，会以为没生效。
	//
	// Action 读规则对象自己的属性，不从规则名里猜：名字带 "block" 的放行规则、名字
	// 不带 block 的阻止规则都真实存在，靠名字猜出来的「允许/阻止」是错的。
	const q = `Get-NetFirewallRule -Direction Inbound -Enabled True` +
		` | Sort-Object @{ Expression = { if ($_.DisplayName -like 'OpsMini-*') { 0 } else { 1 } } }, DisplayName` +
		` | Select-Object -First 60` +
		` | ForEach-Object { $p = $_ | Get-NetFirewallPortFilter;` +
		` [pscustomobject]@{ Name=$_.DisplayName; Proto=[string]$p.Protocol; Port=[string]($p.LocalPort -join ','); Action=[string]$_.Action } }`
	var rows []row
	if err := psJSONWithin(q, &rows, firewallQueryTimeout); err != nil {
		return nil, err
	}

	out := make([]FirewallRule, 0, len(rows))
	for _, r := range rows {
		name := strings.TrimSpace(r.Name)
		if name == "" {
			continue
		}
		action := "allow"
		if strings.EqualFold(strings.TrimSpace(r.Action), "block") {
			action = "deny"
		}
		out = append(out, FirewallRule{
			Ref:     name,
			Proto:   strings.ToLower(r.Proto),
			Port:    r.Port,
			Action:  action,
			Raw:     fmt.Sprintf("%s %s/%s", name, strings.ToLower(r.Proto), r.Port),
			Managed: strings.HasPrefix(name, windowsRulePrefix),
		})
	}
	return out, nil
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

// windowsDeleteRule removes a panel-created rule by its display name (Ref from the list).
//
// 只允许删 OpsMini- 前缀的规则。按名字删除内置规则既容易误触又难以复原，
// 而面板能管理的本来就只是自己建的那几条；其余请到系统防火墙里改。
func windowsDeleteRule(ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return errors.New("规则名不能为空")
	}
	if !strings.HasPrefix(strings.ToLower(ref), strings.ToLower(windowsRulePrefix)) {
		return fmt.Errorf("只能删除面板创建的规则（%s 开头）；内置规则请到 Windows 防火墙中管理", windowsRulePrefix)
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
