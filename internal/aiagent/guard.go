package aiagent

import (
	"fmt"
	"regexp"
	"strings"
)

// Policy decides how a specific tool call must be handled.
type Policy int

const (
	// PolicyAuto executes the call immediately.
	PolicyAuto Policy = iota
	// PolicyConfirm suspends the run until a human approves the call.
	PolicyConfirm
	// PolicyDeny refuses the call outright (never executed, even with approval).
	PolicyDeny
)

// String returns the stable identifier used by the API.
func (p Policy) String() string {
	switch p {
	case PolicyAuto:
		return "auto"
	case PolicyConfirm:
		return "confirm"
	case PolicyDeny:
		return "deny"
	}
	return "unknown"
}

// Rule is a compiled forbidden-pattern rule.
type Rule struct {
	Name    string
	Reason  string
	Pattern *regexp.Regexp
}

// forbiddenArgs holds L3 patterns matched against the string arguments of a tool
// call. These are destructive commands that must never be executed by the agent,
// with or without human approval.
var forbiddenArgs = []Rule{
	{Name: "rm-root", Reason: "删除根目录或系统关键目录", Pattern: regexp.MustCompile(`(?i)\brm\s+(-[a-z]*\s+)*(-[a-z]*[rf][a-z]*\s+)+(/(\s|$|\*)|/(bin|boot|dev|etc|lib|lib64|proc|root|run|sbin|srv|sys|usr|var)(\s|$|/))`)},
	{Name: "rm-wildcard-root", Reason: "通配符删除根目录内容", Pattern: regexp.MustCompile(`(?i)\brm\s+(-[a-z]*\s+)*/\*`)},
	{Name: "mkfs", Reason: "格式化文件系统", Pattern: regexp.MustCompile(`(?i)\bmkfs(\.\w+)?\b`)},
	{Name: "dd-disk", Reason: "直接写入块设备", Pattern: regexp.MustCompile(`(?i)\bdd\b[^\n]*\bof=/dev/(sd|hd|nvme|vd|xvd|mmcblk|disk)`)},
	{Name: "redirect-disk", Reason: "重定向覆盖块设备", Pattern: regexp.MustCompile(`>\s*/dev/(sd|hd|nvme|vd|xvd|mmcblk|disk)`)},
	{Name: "power", Reason: "关机 / 重启 / 停机", Pattern: regexp.MustCompile(`(?i)(^|[\s;&|])(shutdown|reboot|halt|poweroff)(\s|$|;|&|\|)|\binit\s+0\b|\binit\s+6\b|systemctl\s+(poweroff|reboot|halt)`)},
	{Name: "chmod-root", Reason: "递归修改根目录权限", Pattern: regexp.MustCompile(`(?i)\bchmod\s+(-[a-z]*\s+)*(-R\s+)?[0-7]{3,4}\s+/(\s|$|\*)`)},
	{Name: "chown-root", Reason: "递归修改根目录属主", Pattern: regexp.MustCompile(`(?i)\bchown\s+(-[a-z]*\s+)*(-R\s+)?\S+\s+/(\s|$|\*)`)},
	{Name: "fork-bomb", Reason: "fork 炸弹", Pattern: regexp.MustCompile(`:\s*\(\s*\)\s*\{.*\}\s*;\s*:`)},
	{Name: "flush-iptables", Reason: "清空防火墙规则", Pattern: regexp.MustCompile(`(?i)\b(iptables|ip6tables)\s+(-F|-X|--flush|--delete-chain)\b|\bnft\s+flush\s+ruleset`)},
	{Name: "overwrite-passwd", Reason: "覆盖系统账户/认证文件", Pattern: regexp.MustCompile(`>\s*/etc/(passwd|shadow|sudoers|fstab|hostname)`)},
	{Name: "history-wipe", Reason: "清理命令历史痕迹", Pattern: regexp.MustCompile(`(?i)(^|[\s;&|])(history\s+-c|rm\s+[^\n]*\.bash_history)`)},
	{Name: "kernel-module", Reason: "卸载内核模块", Pattern: regexp.MustCompile(`(?i)\brmmod\s+`)},
	{Name: "kill-init", Reason: "杀死 1 号进程", Pattern: regexp.MustCompile(`(?i)\bkill\s+-9\s+1\b`)},
}

// Guard enforces the agent's safety rails: hard-forbidden command patterns plus
// protection of the panel itself (binary, database and listening port).
type Guard struct {
	// DataDir returns the panel data directory (holds the binary, the SQLite
	// database and the skills). It is resolved lazily so configuration changes
	// take effect immediately.
	DataDir func() string
	// PanelPort is the port the panel listens on; rules that would cut off access
	// to the panel are refused.
	PanelPort int
}

// NewGuard creates a Guard.
func NewGuard(dataDir func() string, panelPort int) *Guard {
	return &Guard{DataDir: dataDir, PanelPort: panelPort}
}

// Scan inspects every string value of the tool arguments and returns a blocking
// reason when the call matches a forbidden pattern. The second return value is
// the matched rule name (empty when the call is clean).
func (g *Guard) Scan(t *Tool, args map[string]any) (reason, rule string) {
	values := collectStrings(args, 0)
	for _, v := range values {
		if strings.TrimSpace(v) == "" {
			continue
		}
		for _, r := range forbiddenArgs {
			if r.Pattern.MatchString(v) {
				return fmt.Sprintf("该操作被安全策略硬禁止（%s）：%s", r.Name, r.Reason), r.Name
			}
		}
		if why := g.panelBreach(t, v); why != "" {
			return why, "panel-protection"
		}
	}
	return "", ""
}

// panelBreach detects attempts to damage the panel itself: deleting or
// overwriting its binary/database, or closing its listening port.
//
// Read-only tools may still inspect the data directory (e.g. dir_usage on the
// data volume); anything that could modify it is refused.
func (g *Guard) panelBreach(t *Tool, v string) string {
	lower := strings.ToLower(v)
	readOnly := t != nil && t.Risk == RiskReadonly

	// Panel data directory: never modify, never delete.
	if dir := g.dataDir(); dir != "" && strings.Contains(v, dir) {
		destructive := regexp.MustCompile(`(?i)\brm\b|\bmv\b|\bcp\b|>\s*|>>\s*|\btruncate\b`).MatchString(lower)
		if destructive || !readOnly {
			return "该操作会破坏 OpsMini 面板自身（数据目录 " + dir + "），已被安全策略拒绝"
		}
	}

	// Panel listening port: refuse explicit firewall blocks on it.
	if g.PanelPort > 0 {
		port := fmt.Sprintf("%d", g.PanelPort)
		block := regexp.MustCompile(`(?i)(ufw\s+deny\s+` + port + `\b)|(--dport\s+` + port + `\b[^\n]*(-j\s+(DROP|REJECT)))|(firewall-cmd[^\n]*--remove-port=` + port + `)`)
		if block.MatchString(v) {
			return fmt.Sprintf("该操作会阻断面板端口 %s，导致无法访问 OpsMini，已被安全策略拒绝", port)
		}
	}
	return ""
}

// dataDir resolves the configured data directory.
func (g *Guard) dataDir() string {
	if g.DataDir == nil {
		return ""
	}
	return strings.TrimRight(g.DataDir(), "/")
}

// collectStrings flattens the string leaves of a decoded JSON value.
func collectStrings(v any, depth int) []string {
	if depth > 6 {
		return nil
	}
	switch t := v.(type) {
	case string:
		return []string{t}
	case []any:
		var out []string
		for _, item := range t {
			out = append(out, collectStrings(item, depth+1)...)
		}
		return out
	case map[string]any:
		var out []string
		for _, item := range t {
			out = append(out, collectStrings(item, depth+1)...)
		}
		return out
	}
	return nil
}

// PolicyResolver decides the policy for a tool given the administrator's
// per-tool overrides. It returns an empty string when there is no override, in
// which case the risk level decides.
type PolicyResolver func(toolName string) string

// PolicyFor maps a tool's risk level (and optional administrator override) to
// the policy the loop must apply.
func PolicyFor(t *Tool, override string) Policy {
	// An explicit override always wins, except that L3 can never be enabled.
	switch override {
	case "deny":
		return PolicyDeny
	case "confirm":
		return PolicyConfirm
	case "auto":
		if t.Risk == RiskForbidden {
			return PolicyDeny
		}
		return PolicyAuto
	}
	switch t.Risk {
	case RiskReadonly:
		return PolicyAuto
	case RiskLowWrite:
		// Safe default: low-risk writes still ask the first time. Administrators
		// can opt individual tools into unattended execution.
		return PolicyConfirm
	case RiskHighWrite:
		return PolicyConfirm
	default:
		return PolicyDeny
	}
}
