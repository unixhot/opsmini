package safety

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/opsmini/opsmini/internal/model"
)

// ActionCheck is the verification of one executed write action.
type ActionCheck struct {
	Tool   string `json:"tool"`
	Target string `json:"target"`
	OK     bool   `json:"ok"`
	// Skipped means the check could not be decided (no data, unsupported tool).
	// It counts as "not verified" but must not be reported as a failure: claiming
	// an action failed when we simply cannot see is worse than saying nothing.
	Skipped bool   `json:"skipped"`
	Detail  string `json:"detail"`
}

// VerifyActions re-checks every write action this incident actually executed.
//
// The point: "the tool returned success" is not evidence that the host changed.
// quarantine_file can report 已隔离 while the file is still there (wrong path,
// permission, race), block_ip can return OK while no rule was inserted. This
// turns the tool's own claim into a second, independent observation.
func (m *Manager) VerifyActions(ctx context.Context, incidentID string) (bool, []ActionCheck) {
	if m.DB == nil || incidentID == "" {
		return true, nil
	}
	var logs []model.ActionLog
	// Only actions that actually ran and can change the host. A "shadow" verdict
	// means nothing happened, and read-only tools have nothing to verify.
	m.DB.Where("incident_id = ? AND verdict = ? AND risk IN ?", incidentID,
		model.ActionAllow, []string{"low_write", "high_write", "forbidden"}).
		Order("id asc").Find(&logs)
	if len(logs) == 0 {
		return true, nil
	}

	checks := make([]ActionCheck, 0, len(logs))
	ok := true
	for _, l := range logs {
		c := m.verifyOne(ctx, l)
		if !c.OK && !c.Skipped {
			ok = false
		}
		checks = append(checks, c)
	}
	return ok, checks
}

type actionArgs struct {
	Path   string `json:"path"`
	IP     string `json:"ip"`
	PID    int    `json:"pid"`
	Name   string `json:"name"`
	Unit   string `json:"unit"`
	Action string `json:"action"`
	Reason string `json:"reason"`
	// keep_lines is what truncate_log promises to leave behind.
	KeepLines int `json:"keep_lines"`
}

func (m *Manager) verifyOne(ctx context.Context, l model.ActionLog) ActionCheck {
	var a actionArgs
	_ = json.Unmarshal([]byte(l.Args), &a)
	c := ActionCheck{Tool: l.Tool, Target: a.Path + a.IP}

	switch l.Tool {
	case "quarantine_file":
		c.Target = a.Path
		if a.Path == "" {
			return skip(c, "参数里没有路径")
		}
		if _, err := os.Stat(a.Path); err == nil {
			return fail(c, "文件仍在原位置："+a.Path)
		} else if !os.IsNotExist(err) {
			return skip(c, "无法判断："+err.Error())
		}
		// 文件不在了还不够：隔离区必须有对应记录，否则可能是被别的进程删掉的。
		var n int64
		m.DB.Model(&model.QuarantineItem{}).
			Where("original_path = ? AND created_at > ?", a.Path, l.CreatedAt.Add(-2*oneMinute)).
			Count(&n)
		if n == 0 {
			return fail(c, "文件已不存在，但隔离区没有对应记录（可能被其它进程删除）")
		}
		return pass(c, "文件已移入隔离区")

	case "truncate_log":
		c.Target = a.Path
		if a.Path == "" {
			return skip(c, "参数里没有路径")
		}
		st, err := os.Stat(a.Path)
		if err != nil {
			return skip(c, "无法读取文件："+err.Error())
		}
		// 截断后日志会继续增长，所以不比对具体字节数，只看是否远小于"未截断"的规模。
		keep := a.KeepLines
		if keep <= 0 {
			keep = 2000
		}
		// 一行按 512 字节估算上界；超过这个量级说明没截干净。
		if st.Size() > int64(keep)*512 {
			return fail(c, fmt.Sprintf("文件仍有 %.1f MB，超过保留 %d 行的预期规模", float64(st.Size())/1048576, keep))
		}
		return pass(c, fmt.Sprintf("文件已缩小到 %.1f KB", float64(st.Size())/1024))

	case "block_ip":
		c.Target = a.IP
		if a.IP == "" {
			return skip(c, "参数里没有 IP")
		}
		if ruleExists(ctx, a.IP) {
			return pass(c, "防火墙规则已存在")
		}
		// 有的环境用 nft 或云安全组，看不到本机规则不代表没生效。
		var n int64
		m.DB.Model(&model.BlockedIP{}).
			Where("ip = ? AND status = ?", a.IP, "active").Count(&n)
		if n > 0 {
			return pass(c, "封禁记录已生效（未在本机 iptables 中确认）")
		}
		return fail(c, "iptables 与封禁记录里都没有该 IP")

	case "freeze_process":
		c.Target = fmt.Sprint(a.PID)
		if a.PID <= 0 {
			return skip(c, "参数里没有 PID")
		}
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", a.PID))
		if err != nil {
			return skip(c, "进程已不存在，无法确认冻结状态")
		}
		// 第三个字段是状态，T/t 表示被 SIGSTOP 停住。
		fields := strings.Fields(string(stat))
		if len(fields) < 3 {
			return skip(c, "无法解析进程状态")
		}
		if fields[2] == "T" || fields[2] == "t" {
			return pass(c, "进程已冻结（状态 T）")
		}
		return fail(c, "进程状态是 "+fields[2]+"，未冻结")

	case "service_control":
		unit := a.Name
		if unit == "" {
			unit = a.Unit
		}
		c.Target = unit
		if unit == "" {
			return skip(c, "参数里没有服务名")
		}
		act := a.Action
		if act == "" {
			act = "restart"
		}
		out, err := exec.CommandContext(ctx, "systemctl", "is-active", unit).Output()
		state := strings.TrimSpace(string(out))
		if err != nil && state == "" {
			return skip(c, "systemctl 不可用："+err.Error())
		}
		if act == "stop" {
			if state == "inactive" || state == "failed" {
				return pass(c, "服务已停止（"+state+"）")
			}
			return fail(c, "服务期望停止，实际 "+state)
		}
		if state == "active" {
			return pass(c, "服务运行中")
		}
		return fail(c, "服务期望运行，实际 "+state)

	case "snapshot_path":
		c.Target = a.Path
		return skip(c, "快照类动作不做二次校验")
	}
	return skip(c, "该动作没有默认校验规则")
}

const oneMinute = 60 * 1e9

func pass(c ActionCheck, d string) ActionCheck { c.OK, c.Detail = true, d; return c }
func fail(c ActionCheck, d string) ActionCheck { c.OK, c.Detail = false, d; return c }
func skip(c ActionCheck, d string) ActionCheck { c.OK, c.Skipped, c.Detail = true, true, d; return c }

// ruleExists looks for a drop/reject rule covering ip in the legacy iptables
// ruleset. nft-only hosts return false here; the caller falls back to the
// blocked_ips table rather than declaring failure.
func ruleExists(ctx context.Context, ip string) bool {
	out, err := exec.CommandContext(ctx, "iptables", "-S").Output()
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, ip) && (strings.Contains(line, "DROP") || strings.Contains(line, "REJECT")) {
			return true
		}
	}
	return false
}

// SummarizeChecks renders checks as a short human list for the incident summary.
func SummarizeChecks(checks []ActionCheck) string {
	var b strings.Builder
	for _, c := range checks {
		mark := "✓"
		if c.Skipped {
			mark = "?"
		} else if !c.OK {
			mark = "✗"
		}
		b.WriteString(fmt.Sprintf("%s %s %s：%s\n", mark, c.Tool, filepath.Base(c.Target), c.Detail))
	}
	return strings.TrimSpace(b.String())
}
