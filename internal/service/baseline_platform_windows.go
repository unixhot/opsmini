//go:build windows

package service

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// baselineUnixOnly selects which item set BaselineService runs. Windows has its
// own set because every Linux item (sshd_config, /root permissions, PAM) has no
// counterpart here.
const baselineUnixOnly = false

// regDWORD reads a DWORD, returning a sentinel when the value or key is absent.
func regDWORD(path, name string) (uint32, bool) {
	k, err := registry.OpenKey(registry.LOCAL_MACHINE, path, registry.QUERY_VALUE)
	if err != nil {
		return 0, false
	}
	defer k.Close()
	v, _, err := k.GetIntegerValue(name)
	if err != nil {
		return 0, false
	}
	return uint32(v), true
}

// platformItems returns the Windows security baseline.
//
// Every item is a registry read or a LocalAccounts query, because both are
// language-neutral. `net accounts` and `netsh advfirewall show` were rejected
// for this: their output is localised, so parsing them breaks on a Chinese
// Windows — the exact platform this baseline exists for.
func platformItems() []checkItem {
	return []checkItem{
		{
			key: "uac_enabled", category: "account", title: "用户账户控制（UAC）已启用", weight: 3,
			run: func() (string, string, string) {
				v, ok := regDWORD(`SOFTWARE\Microsoft\Windows\CurrentVersion\Policies\System`, "EnableLUA")
				if !ok {
					return "warn", "无法读取 UAC 配置", "确认注册表 EnableLUA 存在且为 1"
				}
				if v == 1 {
					return "pass", "EnableLUA=1", ""
				}
				return "fail", "EnableLUA=0", "设置 → 账户 → 更改用户账户控制设置，调到默认级别后重启"
			},
		},
		{
			key: "smb1_disabled", category: "network", title: "SMBv1 协议已禁用", weight: 3,
			run: func() (string, string, string) {
				v, ok := regDWORD(`SYSTEM\CurrentControlSet\Services\LanmanServer\Parameters`, "SMB1")
				if !ok {
					// Absent means the default; on Win10 1709+ the feature is off.
					return "warn", "未显式配置 SMB1（系统默认可能已禁用）", "禁用 SMBv1 功能：Disable-WindowsOptionalFeature -Online -FeatureName SMB1Protocol"
				}
				if v == 0 {
					return "pass", "SMB1=0", ""
				}
				return "fail", "SMB1=1", "Disable-WindowsOptionalFeature -Online -FeatureName SMB1Protocol -NoRestart"
			},
		},
		{
			key: "firewall_enabled", category: "network", title: "Windows 防火墙已启用", weight: 3,
			run: func() (string, string, string) {
				profiles := []struct{ path, name string }{
					{`SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\DomainProfile`, "域"},
					{`SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\StandardProfile`, "专用"},
					{`SYSTEM\CurrentControlSet\Services\SharedAccess\Parameters\FirewallPolicy\PublicProfile`, "公用"},
				}
				var on, total int
				var detail []string
				for _, p := range profiles {
					v, ok := regDWORD(p.path, "EnableFirewall")
					total++
					if ok && v == 1 {
						on++
					}
					state := "未启用"
					if ok && v == 1 {
						state = "已启用"
					}
					detail = append(detail, p.name+"："+state)
				}
				d := strings.Join(detail, "，")
				if on == total {
					return "pass", d, ""
				}
				if on == 0 {
					return "fail", d, "Set-NetFirewallProfile -Profile Domain,Public,Private -Enabled True"
				}
				return "warn", d, "部分配置文件未启用，建议全部开启"
			},
		},
		{
			key: "rdp_nla", category: "network", title: "远程桌面已关闭或强制网络级认证", weight: 2,
			run: func() (string, string, string) {
				deny, ok := regDWORD(`SYSTEM\CurrentControlSet\Control\Terminal Server`, "fDenyTSConnections")
				if ok && deny == 1 {
					return "pass", "远程桌面已关闭", ""
				}
				nla, ok := regDWORD(`SYSTEM\CurrentControlSet\Control\Terminal Server\WinStations\RDP-Tcp`, "UserAuthentication")
				if ok && nla == 1 {
					return "pass", "远程桌面已开启，且强制 NLA", ""
				}
				return "warn", "远程桌面已开启但未强制 NLA", "系统属性 → 远程 → 仅允许运行使用网络级别身份验证的远程桌面连接"
			},
		},
		{
			key: "defender_realtime", category: "malware", title: "Defender 实时防护已开启", weight: 3,
			run: func() (string, string, string) {
				v, ok := regDWORD(`SOFTWARE\Microsoft\Windows Defender\Real-Time Protection`, "DisableRealtimeMonitoring")
				if !ok {
					// Tamper protection hides the key; absence is not a failure.
					return "warn", "无法读取实时防护状态（可能启用了篡改保护）", "在 Windows 安全中心确认「实时保护」为开启"
				}
				if v == 0 {
					return "pass", "实时防护已开启", ""
				}
				return "fail", "实时防护已关闭", "Windows 安全中心 → 病毒和威胁防护 → 管理设置 → 开启实时保护"
			},
		},
		{
			key: "auto_update", category: "patch", title: "Windows 自动更新未被禁用", weight: 2,
			run: func() (string, string, string) {
				v, ok := regDWORD(`SOFTWARE\Policies\Microsoft\Windows\WindowsUpdate\AU`, "NoAutoUpdate")
				if ok && v == 1 {
					return "fail", "组策略已禁用自动更新", "删除 NoAutoUpdate 策略，或在设置中启用自动更新"
				}
				return "pass", "自动更新未被策略禁用", ""
			},
		},
		{
			key: "guest_disabled", category: "account", title: "Guest 账户已禁用", weight: 2,
			run: func() (string, string, string) {
				var raw []struct {
					Name    string `json:"Name"`
					Enabled bool   `json:"Enabled"`
				}
				if err := psJSON(`Get-LocalUser -Name Guest -ErrorAction SilentlyContinue | ForEach-Object { [pscustomobject]@{ Name=$_.Name; Enabled=$_.Enabled } }`, &raw); err != nil || len(raw) == 0 {
					return "warn", "无法读取 Guest 账户状态", "services.msc 之外的账户管理：net user guest 查看"
				}
				if raw[0].Enabled {
					return "fail", "Guest 账户已启用", "net user guest /active:no"
				}
				return "pass", "Guest 账户已禁用", ""
			},
		},
		{
			key: "admin_group_size", category: "account", title: "本地管理员组成员不超过 3 个", weight: 2,
			run: func() (string, string, string) {
				var raw []struct {
					Name string `json:"Name"`
				}
				if err := psJSON(`Get-LocalGroupMember -Group Administrators -ErrorAction SilentlyContinue | ForEach-Object { [pscustomobject]@{ Name=$_.Name } }`, &raw); err != nil {
					return "warn", "无法读取管理员组成员", "在「计算机管理 → 本地用户和组」中检查"
				}
				n := len(raw)
				if n == 0 {
					return "warn", "未读取到管理员组成员", "在「计算机管理 → 本地用户和组」中检查"
				}
				if n <= 3 {
					return "pass", fmt.Sprintf("管理员组 %d 个成员", n), ""
				}
				return "warn", fmt.Sprintf("管理员组 %d 个成员，偏多", n), "按最小权限原则收敛管理员组成员"
			},
		},
	}
}
