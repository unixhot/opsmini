package tools

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/opsmini/opsmini/internal/aiagent"
)

// Response tools: the reversible replacements for destructive operations.
//
// Everything here is L1 and reversible by design. The agent never gets a "delete
// this file" primitive in event-triggered mode; it gets "quarantine this file",
// which can be undone with one call. That is the whole point of this file.

// responseTools returns the safety-backed action tools.
func responseTools(d Deps) []aiagent.Tool {
	if d.Safety == nil {
		return nil
	}
	return []aiagent.Tool{
		quarantineFileTool(d),
		restoreQuarantineTool(d),
		freezeProcessTool(d),
		resumeProcessTool(d),
		snapshotPathTool(d),
		restoreSnapshotTool(d),
		blockIPTool(d),
		unblockIPTool(d),
		truncateLogTool(d),
	}
}

func quarantineFileTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:  "quarantine_file",
		Title: "隔离文件（可还原）",
		Description: "把可疑文件移动到隔离区而不是删除：保留原始路径、权限、属主与 sha256，可用 restore_quarantine 一键还原。" +
			"这是处置 WebShell 等落地文件的标准动作，优先于任何删除操作。",
		Risk: aiagent.RiskLowWrite,
		Schema: schema(map[string]any{
			"path":   pStr("要隔离的文件绝对路径"),
			"reason": pStr("隔离原因（写进审计日志）"),
		}, "path"),
		Preview: func(args map[string]any) string {
			return "隔离文件 " + strDef(args, "path", "")
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			path := strDef(args, "path", "")
			reason := strDef(args, "reason", "由 Agent 判定为可疑文件")
			if v, rule, why := d.Safety.CheckArg("quarantine_file", args); v != "allow" {
				return failed("已拒绝（%s）：%s", rule, why), nil
			}
			item, err := d.Safety.Quarantine(ctx.SessionID, path, reason)
			if err != nil {
				return failed("隔离失败：%v", err), nil
			}
			return textResult(fmt.Sprintf("已隔离 %s → %s（sha256 %s，%d 字节）。还原：restore_quarantine id=%d",
				item.OriginalPath, item.StoredPath, short(item.SHA256), item.Size, item.ID)), nil
		},
	}
}

func restoreQuarantineTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "restore_quarantine",
		Title:       "还原隔离文件",
		Description: "把之前隔离的文件放回原路径（原路径已存在同名文件时拒绝覆盖）。用于误判回退。",
		Risk:        aiagent.RiskLowWrite,
		Schema: schema(map[string]any{
			"id": pInt("隔离记录 ID（quarantine_file 返回）"),
		}, "id"),
		Preview: func(args map[string]any) string {
			return fmt.Sprintf("还原隔离记录 %v", args["id"])
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			item, err := d.Safety.RestoreQuarantine(uint(intArg(args, "id", 0)))
			if err != nil {
				return failed("还原失败：%v", err), nil
			}
			return textResult("已还原到 " + item.OriginalPath), nil
		},
	}
}

func freezeProcessTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:  "freeze_process",
		Title: "冻结进程（可恢复）",
		Description: "向进程发送 SIGSTOP 冻结运行（不是 kill）：现场保留、可 resume_process 恢复。" +
			"处置可疑进程时先用这个，杀进程属于高危操作必须人工确认。",
		Risk: aiagent.RiskLowWrite,
		Schema: schema(map[string]any{
			"pid":    pInt("进程 PID"),
			"reason": pStr("冻结原因"),
		}, "pid"),
		Preview: func(args map[string]any) string {
			return fmt.Sprintf("冻结进程 pid=%d", intArg(args, "pid", 0))
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			pid := intArg(args, "pid", 0)
			if v, rule, why := d.Safety.CheckArg("freeze_process", args); v != "allow" {
				return failed("已拒绝（%s）：%s", rule, why), nil
			}
			if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
				return failed("冻结失败：%v", err), nil
			}
			return textResult(fmt.Sprintf("已冻结 pid=%d（SIGSTOP，可用 resume_process 恢复）", pid)), nil
		},
	}
}

func resumeProcessTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "resume_process",
		Title:       "恢复冻结进程",
		Description: "向被冻结的进程发送 SIGCONT 恢复运行。",
		Risk:        aiagent.RiskLowWrite,
		Schema: schema(map[string]any{
			"pid": pInt("进程 PID"),
		}, "pid"),
		Preview: func(args map[string]any) string {
			return fmt.Sprintf("恢复进程 pid=%d", intArg(args, "pid", 0))
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			pid := intArg(args, "pid", 0)
			if err := syscall.Kill(pid, syscall.SIGCONT); err != nil {
				return failed("恢复失败：%v", err), nil
			}
			return textResult(fmt.Sprintf("已恢复 pid=%d", pid)), nil
		},
	}
}

func snapshotPathTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "snapshot_path",
		Title:       "快照文件（可回滚）",
		Description: "修改文件或配置前先做快照，失败时用 restore_snapshot 回滚。改配置前必须先调用本工具。",
		Risk:        aiagent.RiskLowWrite,
		Schema: schema(map[string]any{
			"path": pStr("要快照的文件绝对路径"),
			"kind": pEnum("快照类型", "file", "config"),
		}, "path"),
		Preview: func(args map[string]any) string {
			return "快照 " + strDef(args, "path", "")
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			rec, err := d.Safety.Snapshot(ctx.SessionID, strDef(args, "path", ""), strDef(args, "kind", "file"))
			if err != nil {
				return failed("快照失败：%v", err), nil
			}
			return textResult(fmt.Sprintf("已快照 %s（id=%d，sha256 %s）", rec.Path, rec.ID, short(rec.SHA256))), nil
		},
	}
}

func restoreSnapshotTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "restore_snapshot",
		Title:       "回滚快照",
		Description: "把文件回滚到指定快照。验证未通过时用它撤销上一步修改。",
		Risk:        aiagent.RiskLowWrite,
		Schema: schema(map[string]any{
			"id": pInt("快照 ID"),
		}, "id"),
		Preview: func(args map[string]any) string {
			return fmt.Sprintf("回滚快照 %v", args["id"])
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			rec, err := d.Safety.RestoreSnapshot(uint(intArg(args, "id", 0)))
			if err != nil {
				return failed("回滚失败：%v", err), nil
			}
			return textResult("已回滚 " + rec.Path), nil
		},
	}
}

func blockIPTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:  "block_ip",
		Title: "临时封禁 IP（带 TTL）",
		Description: "在防火墙 INPUT 链临时丢弃来源 IP，默认 60 分钟后自动解封。永久封禁必须人工操作。" +
			"封禁前请确认该 IP 不是运维出口、不是本机地址。",
		Risk: aiagent.RiskLowWrite,
		Schema: schema(map[string]any{
			"ip":          pStr("要封禁的 IPv4 地址"),
			"ttl_minutes": pInt("封禁时长（分钟，默认 60，最大 1440）"),
			"reason":      pStr("封禁原因"),
		}, "ip"),
		Preview: func(args map[string]any) string {
			return "临时封禁 " + strDef(args, "ip", "") + " " + fmt.Sprintf("%d 分钟", ttld(args))
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			if v, rule, why := d.Safety.CheckArg("block_ip", args); v != "allow" {
				return failed("已拒绝（%s）：%s", rule, why), nil
			}
			ttl := ttld(args)
			rec, err := d.Safety.BlockIP(ctx.SessionID, strDef(args, "ip", ""),
				timeDur(ttl), strDef(args, "reason", "Agent 判定为可疑来源"))
			if err != nil {
				return failed("封禁失败：%v", err), nil
			}
			return textResult(fmt.Sprintf("已封禁 %s，解封时间 %s（记录 id=%d）",
				rec.IP, rec.ExpiresAt.Format("2006-01-02 15:04:05"), rec.ID)), nil
		},
	}
}

func unblockIPTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "unblock_ip",
		Title:       "解封 IP",
		Description: "删除由 block_ip 添加的临时封禁规则。",
		Risk:        aiagent.RiskLowWrite,
		Schema: schema(map[string]any{
			"ip": pStr("要解封的 IPv4 地址"),
		}, "ip"),
		Preview: func(args map[string]any) string {
			return "解封 " + strDef(args, "ip", "")
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			ip := strDef(args, "ip", "")
			if err := d.Safety.UnblockIP(ip); err != nil {
				return failed("解封失败：%v", err), nil
			}
			return textResult("已解封 " + ip), nil
		},
	}
}

func truncateLogTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:  "truncate_log",
		Title: "截断日志（保留尾部）",
		Description: "把日志截断到只保留最后 N 行，截断前自动做快照。不会删除文件本身，" +
			"因此持有该文件的进程不会拿到 EBADF。",
		Risk: aiagent.RiskLowWrite,
		Schema: schema(map[string]any{
			"path":       pStr("日志文件绝对路径"),
			"keep_lines": pInt("保留尾部行数（默认 2000）"),
		}, "path"),
		Preview: func(args map[string]any) string {
			return "截断日志 " + strDef(args, "path", "")
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			path := strDef(args, "path", "")
			keep := intArg(args, "keep_lines", 0)
			if keep <= 0 {
				keep = 2000
			}
			if v, rule, why := d.Safety.CheckArg("truncate_log", args); v != "allow" {
				return failed("已拒绝（%s）：%s", rule, why), nil
			}
			snap, err := d.Safety.Snapshot(ctx.SessionID, path, "file")
			if err != nil {
				return failed("截断前快照失败，已放弃操作：%v", err), nil
			}
			lines, err := tailFileLines(path, keep)
			if err != nil {
				return failed("读取日志失败：%v", err), nil
			}
			if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
				return failed("写回失败：%v", err), nil
			}
			return textResult(fmt.Sprintf("已截断 %s，保留末 %d 行；快照 id=%d 可回滚", path, len(lines), snap.ID)), nil
		},
	}
}

// tailFileLines reads the last n lines without loading the whole file.
func tailFileLines(path string, n int) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	const window = 1 << 20
	if _, err := f.Seek(-window, 2); err != nil {
		if _, err := f.Seek(0, 0); err != nil {
			return nil, err
		}
	}
	buf := make([]byte, window)
	read, _ := f.Read(buf)
	lines := strings.Split(strings.TrimRight(string(buf[:read]), "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return lines, nil
}

func short(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// timeDur converts minutes to a duration.
func timeDur(minutes int) time.Duration { return time.Duration(minutes) * time.Minute }

func ttld(args map[string]any) int {
	n := intArg(args, "ttl_minutes", 0)
	if n <= 0 {
		return 60
	}
	if n > 1440 {
		return 1440
	}
	return n
}
