package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ErrNeedRoot indicates firewall write operations require root privileges.
var ErrNeedRoot = errors.New("需要 root 权限执行防火墙操作")

// FirewallRule is a unified structured firewall rule.
type FirewallRule struct {
	Ref    string `json:"ref"` // rule number (used for deletion)
	Proto  string `json:"proto"`
	Port   string `json:"port"`
	Action string `json:"action"` // allow/deny
	Raw    string `json:"raw"`
	// Managed marks a rule the panel itself created, and therefore the only kind
	// it may delete. Windows ships hundreds of built-in rules; deleting one of
	// them by name is both easy to trigger and hard to undo.
	Managed bool `json:"managed"`
}

// FirewallRules holds the firewall status and rule list.
type FirewallRules struct {
	Backend string         `json:"backend"`
	Enabled bool           `json:"enabled"`
	Rules   []FirewallRule `json:"rules"`
}

// FirewallService reads and writes the firewall (ufw / firewalld / iptables).
type FirewallService struct {
	sys       *SystemService
	panelPort int
}

// NewFirewallService creates a FirewallService. panelPort is the panel's own listening
// port (protected from being closed).
func NewFirewallService(sys *SystemService, panelPort int) *FirewallService {
	return &FirewallService{sys: sys, panelPort: panelPort}
}

// Status returns the firewall status and rule list.
func (s *FirewallService) Status() *FirewallRules {
	fw := s.sys.FirewallStatus()
	out := &FirewallRules{Backend: fw.Backend, Enabled: fw.Enabled, Rules: []FirewallRule{}}
	switch fw.Backend {
	case "ufw":
		out.Rules = s.ufwRules()
	case "firewalld":
		out.Rules = s.firewalldRules()
	case "iptables":
		out.Rules = s.iptablesRules()
	case "windows":
		out.Rules = windowsFirewallRules()
	}
	return out
}

// AllowPort opens a port.
func (s *FirewallService) AllowPort(port int, proto string) error {
	if err := s.guard(port, proto); err != nil {
		return err
	}
	switch s.sys.FirewallStatus().Backend {
	case "ufw":
		_, err := runFirewallCmd("ufw", "allow", fmt.Sprintf("%d/%s", port, proto))
		return err
	case "firewalld":
		if _, err := runFirewallCmd("firewall-cmd", "--permanent", fmt.Sprintf("--add-port=%d/%s", port, proto)); err != nil {
			return err
		}
		_, err := runFirewallCmd("firewall-cmd", "--reload")
		return err
	case "iptables":
		_, err := runFirewallCmd("iptables", "-I", "INPUT", "1", "-p", proto, "--dport", strconv.Itoa(port), "-j", "ACCEPT")
		return err
	case "windows":
		return windowsAllowPort(port, proto)
	default:
		return errors.New("未检测到防火墙后端（ufw/firewalld/iptables/Windows 防火墙）")
	}
}

// DenyPort closes a port.
func (s *FirewallService) DenyPort(port int, proto string) error {
	if err := s.guard(port, proto); err != nil {
		return err
	}
	// panel port protection
	if proto == "tcp" && port == s.panelPort {
		return errors.New("禁止关闭面板自身监听端口")
	}
	switch s.sys.FirewallStatus().Backend {
	case "ufw":
		_, err := runFirewallCmd("ufw", "delete", "allow", fmt.Sprintf("%d/%s", port, proto))
		return err
	case "firewalld":
		if _, err := runFirewallCmd("firewall-cmd", "--permanent", fmt.Sprintf("--remove-port=%d/%s", port, proto)); err != nil {
			return err
		}
		_, err := runFirewallCmd("firewall-cmd", "--reload")
		return err
	case "iptables":
		// 真的插入 DROP，而不是删掉之前那条 ACCEPT。
		//
		// 删 ACCEPT 只是撤销「曾经放行」，链尾策略仍是 ACCEPT 时端口照样通，
		// 而 UI 上写的是「阻止」。先删后插保证重复点击不会堆叠多条 DROP；
		// 插到链首则保证它排在既有 ACCEPT 之前生效。
		_, _ = runFirewallCmd("iptables", "-D", "INPUT", "-p", proto, "--dport", strconv.Itoa(port), "-j", "DROP")
		_, err := runFirewallCmd("iptables", "-I", "INPUT", "1", "-p", proto, "--dport", strconv.Itoa(port), "-j", "DROP")
		return err
	case "windows":
		return windowsDenyPort(port, proto)
	default:
		return errors.New("未检测到防火墙后端（ufw/firewalld/iptables/Windows 防火墙）")
	}
}

// Enable enables the firewall. Before enabling, it always allows the panel's own
// port and SSH (22) to avoid locking the user out.
func (s *FirewallService) Enable() error {
	if err := requireRoot(); err != nil {
		return err
	}
	backend := s.sys.FirewallStatus().Backend
	if backend != "ufw" && backend != "firewalld" && backend != "windows" {
		return errors.New("当前后端不支持面板内启用，请手动配置防火墙")
	}
	// 启用前先放行面板端口 + 远程管理端口，避免启用后无法访问面板。
	// Linux 是 SSH 22，Windows 是 RDP 3389。
	if s.panelPort > 0 {
		if err := s.AllowPort(s.panelPort, "tcp"); err != nil {
			return err
		}
	}
	if err := s.AllowPort(remoteAdminPort(), "tcp"); err != nil {
		return err
	}
	switch backend {
	case "ufw":
		_, err := runFirewallCmd("ufw", "--force", "enable")
		return err
	case "firewalld":
		_, err := runFirewallCmd("systemctl", "enable", "--now", "firewalld")
		return err
	case "windows":
		return windowsEnableFirewall()
	}
	return nil
}

// Disable disables the firewall (not provided for the iptables backend, too destructive).
func (s *FirewallService) Disable() error {
	if err := requireRoot(); err != nil {
		return err
	}
	switch s.sys.FirewallStatus().Backend {
	case "ufw":
		_, err := runFirewallCmd("ufw", "disable")
		return err
	case "firewalld":
		_, err := runFirewallCmd("systemctl", "stop", "firewalld")
		return err
	case "windows":
		return windowsDisableFirewall()
	default:
		return errors.New("当前后端不支持面板内停用（iptables 停用破坏性过高）")
	}
}

// DeleteRule deletes a rule (ref is the rule number).
func (s *FirewallService) DeleteRule(ref string) error {
	if err := requireRoot(); err != nil {
		return err
	}
	switch s.sys.FirewallStatus().Backend {
	case "ufw":
		_, err := runFirewallCmd("ufw", "delete", ref)
		return err
	case "iptables":
		_, err := runFirewallCmd("iptables", "-D", "INPUT", ref)
		return err
	case "windows":
		return windowsDeleteRule(ref)
	default:
		return errors.New("当前后端暂不支持按编号删除规则")
	}
}

// guard validates port and protocol validity.
func (s *FirewallService) guard(port int, proto string) error {
	if err := requireRoot(); err != nil {
		return err
	}
	if port < 1 || port > 65535 {
		return errors.New("端口范围须在 1-65535")
	}
	if proto != "tcp" && proto != "udp" {
		return errors.New("协议仅支持 tcp/udp")
	}
	return nil
}

// requireRoot returns an error unless the process has the privileges firewall
// management needs: uid 0 on Unix, an elevated token on Windows. `os.Geteuid()`
// returns -1 on Windows, so the old check rejected every write there.
func requireRoot() error {
	if !isPrivileged() {
		return ErrNeedRoot
	}
	return nil
}

// runFirewallCmd runs a firewall command with a 10s timeout and returns the output.
//
// 输出按平台解码：netsh 这类原生程序在中文 Windows 上写的是 ANSI 代码页字节，
// 直接当 UTF-8 用会把报错文本变成乱码（见 decodeCmdOutput）。
func runFirewallCmd(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(decodeCmdOutput(out))
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("%s %s 失败：%s", name, strings.Join(args, " "), msg)
	}
	return strings.TrimSpace(decodeCmdOutput(out)), nil
}

// ---- rule parsing (best-effort) ----

// ufwRules parses the "ufw status" output into firewall rules (best-effort).
func (s *FirewallService) ufwRules() []FirewallRule {
	out, _ := runFirewallCmd("ufw", "status")
	var rules []FirewallRule
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Status") || strings.HasPrefix(line, "To") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 3 {
			rules = append(rules, FirewallRule{
				Ref: fields[0], Action: strings.ToLower(fields[1]), Port: fields[2], Proto: "", Raw: line,
			})
		}
	}
	return rules
}

// firewalldRules parses "firewall-cmd --list-ports" output into firewall rules (best-effort).
func (s *FirewallService) firewalldRules() []FirewallRule {
	out, _ := runFirewallCmd("firewall-cmd", "--list-ports")
	var rules []FirewallRule
	for _, p := range strings.Fields(out) {
		parts := strings.Split(p, "/")
		if len(parts) == 2 {
			rules = append(rules, FirewallRule{Port: parts[0], Proto: parts[1], Action: "allow", Raw: p})
		}
	}
	return rules
}

// iptablesRules parses "iptables -L INPUT -n --line-numbers" output into firewall rules (best-effort).
func (s *FirewallService) iptablesRules() []FirewallRule {
	out, _ := runFirewallCmd("iptables", "-L", "INPUT", "-n", "--line-numbers")
	var rules []FirewallRule
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "Chain") || strings.HasPrefix(line, "num") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 1 {
			action := "allow"
			if len(fields) >= 4 {
				action = strings.ToLower(fields[len(fields)-2])
			}
			rules = append(rules, FirewallRule{Ref: fields[0], Action: action, Raw: line})
		}
	}
	return rules
}
