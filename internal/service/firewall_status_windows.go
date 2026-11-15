//go:build windows

package service

import (
	"fmt"
	"strings"
)

// detectFirewall asks the Windows Firewall for the state of each profile.
//
// `netsh advfirewall show allprofiles state` prints localised labels, so parsing
// it breaks on a Chinese system — the exact environment this feature exists for.
// Get-NetFirewallProfile returns objects whose property names are not translated.
func detectFirewall() *FirewallStatus {
	type profile struct {
		Name    string `json:"Name"`
		Enabled any    `json:"Enabled"`
	}
	var rows []profile
	const q = `Get-NetFirewallProfile | Select-Object Name,Enabled`
	if err := psJSON(q, &rows); err != nil {
		return &FirewallStatus{
			Enabled: false,
			Backend: "windows",
			Message: "无法读取 Windows 防火墙状态：" + err.Error(),
		}
	}
	if len(rows) == 0 {
		return &FirewallStatus{Enabled: false, Backend: "windows", Message: "未返回任何防火墙配置"}
	}

	anyOn := false
	var states []string
	for _, r := range rows {
		on := truthy(r.Enabled)
		if on {
			anyOn = true
		}
		states = append(states, fmt.Sprintf("%s=%s", profileLabel(r.Name), onOff(on)))
	}
	msg := "Windows 防火墙：" + strings.Join(states, "，")
	if !anyOn {
		msg = "Windows 防火墙三个配置文件均已关闭"
	}
	return &FirewallStatus{Enabled: anyOn, Backend: "windows", Message: msg}
}

// truthy normalises the Enabled property, which ConvertTo-Json renders as a bool
// on newer builds and as 0/1 on some older ones.
func truthy(v any) bool {
	switch t := v.(type) {
	case bool:
		return t
	case float64:
		return t != 0
	case string:
		return strings.EqualFold(t, "true") || t == "1"
	}
	return false
}

func profileLabel(name string) string {
	switch strings.ToLower(name) {
	case "domain":
		return "域"
	case "private":
		return "专用"
	case "public":
		return "公用"
	}
	return name
}

func onOff(on bool) string {
	if on {
		return "开启"
	}
	return "关闭"
}
