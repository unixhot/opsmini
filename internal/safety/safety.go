// Package safety implements the guard rails around autonomous actions: argument
// level constraints, reversible replacements for destructive operations, action
// budgets and post-action verification.
//
// The design rule is simple: if an action cannot be undone, it must not be
// available to the agent without a human in the loop. That is why files are
// quarantined instead of deleted, processes are frozen instead of killed, and IP
// bans carry a TTL.
package safety

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
)

// Manager owns the safety primitives.
type Manager struct {
	DB        *gorm.DB
	DataDir   func() string
	PanelPort int

	mu     sync.Mutex
	paused bool
	writes map[string]int
	tokens map[string]int
}

// New creates the safety manager.
func New(db *gorm.DB, dataDir func() string, panelPort int) *Manager {
	return &Manager{DB: db, DataDir: dataDir, PanelPort: panelPort,
		writes: map[string]int{}, tokens: map[string]int{}}
}

// --- kill switch -------------------------------------------------------------

// Pause flips the global kill switch. While paused the agent may still read and
// investigate, but every write is refused.
func (m *Manager) Pause(on bool) {
	m.mu.Lock()
	m.paused = on
	m.mu.Unlock()
}

// Paused reports the kill switch state.
func (m *Manager) Paused() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.paused
}

// --- action budget -----------------------------------------------------------

// CountWrite increments the per-incident write counter and reports whether the
// budget still allows the action.
func (m *Manager) CountWrite(incidentID string, max int) (bool, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if incidentID == "" {
		return true, 0
	}
	m.writes[incidentID]++
	used := m.writes[incidentID]
	if max > 0 && used > max {
		return false, used
	}
	return true, used
}

// WritesUsed reports how many writes an incident has consumed.
func (m *Manager) WritesUsed(incidentID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.writes[incidentID]
}

// AddTokens records token usage for an incident.
func (m *Manager) AddTokens(incidentID string, n int) {
	m.mu.Lock()
	m.tokens[incidentID] += n
	m.mu.Unlock()
}

// TokensUsed reports token usage for an incident.
func (m *Manager) TokensUsed(incidentID string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tokens[incidentID]
}

// --- argument policy ---------------------------------------------------------

// protectedPaths must never be modified by an autonomous action.
var protectedPaths = []string{
	"/etc/ssh", "/etc/passwd", "/etc/shadow", "/etc/sudoers", "/etc/sudoers.d",
	"/boot", "/lib/modules", "/data/opsmini", "/usr/lib/systemd", "/etc/systemd",
}

// protectedUnits stop the host from being manageable if touched.
var protectedUnits = []string{
	"sshd", "ssh", "systemd-journald", "systemd-logind", "systemd-networkd",
	"networking", "network", "dbus", "opsmini", "cron", "crond",
}

// protectedProcs are process names the agent must never terminate.
var protectedProcs = []string{
	"systemd", "init", "sshd", "opsmini", "systemd-journal", "kthreadd",
}

// denyCmdPatterns are refused anywhere in an exec_command command line.
var denyCmdPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\brm\s+-[a-z]*r[a-z]*f?\s+/(\s|$)`),
	regexp.MustCompile(`(?i)\bmkfs(\.\w+)?\b`),
	regexp.MustCompile(`(?i)\bdd\s+[^\n]*of=/dev/`),
	regexp.MustCompile(`(?i)\b(shutdown|reboot|poweroff|halt)\b`),
	regexp.MustCompile(`(?i):\(\)\s*\{`),
	regexp.MustCompile(`(?i)\bchmod\s+-R\s+777\s+/(\s|$)`),
	regexp.MustCompile(`(?i)\b(chattr|setfacl)\b.*\s/(\s|$)`),
}

// CheckArg validates a tool call at the argument level. It returns the verdict
// ("allow"/"deny"), the rule that decided, and a human readable reason.
//
// This runs *in addition to* the existing tool-level PolicyFor rules: two
// different exec_command calls have very different blast radii, so the tool name
// alone is not enough to decide.
func (m *Manager) CheckArg(tool string, args map[string]any) (verdict, rule, reason string) {
	if m.Paused() {
		return "deny", "kill-switch", "安全急停已开启：当前只允许只读排查，写操作一律拒绝"
	}
	path := strOf(args, "path")
	if path == "" {
		path = strOf(args, "file")
	}
	if path == "" {
		path = strOf(args, "target")
	}
	if path != "" {
		clean := filepath.Clean(path)
		for _, p := range protectedPaths {
			if clean == p || strings.HasPrefix(clean, p+"/") {
				return "deny", "protected-path:" + p, "路径 " + clean + " 属于受保护路径，需人工操作"
			}
		}
	}
	if unit := strOf(args, "unit"); unit != "" {
		base := strings.TrimSuffix(unit, ".service")
		for _, u := range protectedUnits {
			if base == u {
				return "deny", "protected-unit:" + u, "服务 " + u + " 受保护，禁止由 Agent 操作"
			}
		}
	}
	if pid := intOf(args, "pid"); pid > 0 {
		comm := procName(pid)
		for _, p := range protectedProcs {
			if strings.Contains(comm, p) {
				return "deny", "protected-proc:" + p, "进程 " + comm + " 属于受保护进程"
			}
		}
		if pid == 1 || pid == os.Getpid() {
			return "deny", "protected-proc:self-or-init", "不允许操作 pid 1 或 OpsMini 自身"
		}
	}
	cmd := strOf(args, "command")
	for _, re := range denyCmdPatterns {
		if re.MatchString(cmd) {
			return "deny", "deny-cmd:" + re.String(), "命令命中硬禁止规则，拒绝执行"
		}
	}
	// Never let the agent block the panel port.
	if ip := strOf(args, "ip"); ip != "" && m.PanelPort > 0 {
		if strings.Contains(strOf(args, "reason"), "panel") {
			return "deny", "panel-protect", "不允许针对面板自身的动作"
		}
	}
	return "allow", "", ""
}

func strOf(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		switch t := v.(type) {
		case string:
			return t
		case fmt.Stringer:
			return t.String()
		}
	}
	return ""
}

func intOf(m map[string]any, key string) int {
	if m == nil {
		return 0
	}
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return 0
}

// procName returns the comm of a pid (empty when gone).
func procName(pid int) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/comm", pid))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// IncidentDir returns (and creates) the per-incident working directory.
func (m *Manager) IncidentDir(incidentID, sub string) (string, error) {
	base := m.DataDir()
	if base == "" {
		base = "/var/lib/opsmini/data"
	}
	if incidentID == "" {
		incidentID = "manual"
	}
	dir := filepath.Join(base, "safety", sub, incidentID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// nowPtr is a small helper for the optional timestamps.
func nowPtr() *time.Time {
	t := time.Now()
	return &t
}
