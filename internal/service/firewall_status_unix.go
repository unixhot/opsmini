//go:build !windows

package service

import (
	"os/exec"
	"strings"
)

// detectFirewall probes the Linux/macOS firewall backends in order of specificity.
func detectFirewall() *FirewallStatus {
	// firewalld (RHEL/CentOS/Alibaba Linux, etc.)
	if out, err := exec.Command("firewall-cmd", "--state").Output(); err == nil {
		if strings.TrimSpace(string(out)) == "running" {
			return &FirewallStatus{Enabled: true, Backend: "firewalld", Message: "firewalld 正在运行"}
		}
	}
	// ufw (Ubuntu/Debian)
	if out, err := exec.Command("ufw", "status").Output(); err == nil {
		s := string(out)
		if strings.Contains(s, "Status: active") {
			return &FirewallStatus{Enabled: true, Backend: "ufw", Message: "ufw 正在运行"}
		}
		// ufw 已安装但未启用（Status: inactive），仍识别为 ufw 后端，允许面板内启用
		if strings.Contains(s, "Status: inactive") {
			return &FirewallStatus{Enabled: false, Backend: "ufw", Message: "ufw 已安装但未启用"}
		}
	}
	// iptables: any rules mean enabled
	if out, err := exec.Command("iptables", "-S").Output(); err == nil {
		rules := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(rules) > 0 && strings.TrimSpace(rules[0]) != "" && rules[0] != "-P INPUT ACCEPT" {
			if len(rules) > 20 {
				rules = rules[:20]
			}
			return &FirewallStatus{Enabled: true, Backend: "iptables", Rules: rules, Message: "iptables 规则已配置"}
		}
	}
	return &FirewallStatus{Enabled: false, Backend: "none", Message: "未检测到系统防火墙（firewalld/ufw/iptables 均未启用）"}
}
