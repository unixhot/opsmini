package tools

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/opsmini/opsmini/internal/aiagent"
	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/service"
)

// maxExecOutput caps how much command output is kept.
const maxExecOutput = 256 << 10

// Signals used by the process control tool.
var (
	syscallTerm = syscall.SIGTERM
	syscallKill = syscall.SIGKILL
)

// lowRiskTools returns the L1 tool set: reversible, low-blast-radius write
// operations. They still ask for confirmation unless the administrator opts a
// tool into unattended execution.
func lowRiskTools(d Deps) []aiagent.Tool {
	var out []aiagent.Tool
	out = append(out, cleanTempTool(d))
	if d.Log != nil {
		out = append(out, cleanLogsTool(d))
	}
	out = append(out, serviceControlTool(d))
	if d.Docker != nil {
		out = append(out, dockerControlTool(d))
	}
	if d.Skill != nil {
		out = append(out, installSkillTool(d))
	}
	return out
}

// highRiskTools returns the L2 tool set: disruptive or destructive operations.
// They always require an explicit human approval, and the invoker needs the
// ai.agent.write permission on top of ai.agent.
func highRiskTools(d Deps) []aiagent.Tool {
	var out []aiagent.Tool
	out = append(out,
		execCommandTool(d),
		writeFileTool(d),
		deletePathTool(d),
		modifySSHDTool(d),
		compressDirTool(d),
		cronWriteTool(d),
		killProcessTool(d),
	)
	if d.Firewall != nil {
		out = append(out, firewallRuleTool(d))
	}
	return out
}

// --- L1 ---

// cleanTempTool removes stale files from the system temporary directories.
func cleanTempTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "clean_temp",
		Title:       "清理临时文件",
		Description: "清理 /tmp 与 /var/tmp 中超过指定天数的临时文件（仅限这两个目录）。用于释放磁盘空间。",
		Risk:        aiagent.RiskLowWrite,
		Schema: schema(map[string]any{
			"days":    pInt("只清理修改时间早于 N 天的文件，默认 7"),
			"dry_run": pBool("true 表示只统计不删除，默认 false"),
		}),
		Preview: func(args map[string]any) string {
			if boolArg(args, "dry_run", false) {
				return fmt.Sprintf("统计 /tmp 与 /var/tmp 中超过 %d 天的临时文件（不删除）", intArg(args, "days", 7))
			}
			return fmt.Sprintf("删除 /tmp 与 /var/tmp 中超过 %d 天的临时文件", intArg(args, "days", 7))
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			days := intArg(args, "days", 7)
			if days < 1 {
				days = 1
			}
			dry := boolArg(args, "dry_run", false)
			cutoff := time.Now().AddDate(0, 0, -days)
			roots := []string{"/tmp", "/var/tmp"}
			var (
				total    int64
				removed  int
				failures int
				samples  []string
				details  strings.Builder
			)
			for _, root := range roots {
				entries, err := os.ReadDir(root)
				if err != nil {
					continue
				}
				for _, e := range entries {
					full := filepath.Join(root, e.Name())
					info, ierr := e.Info()
					if ierr != nil {
						continue
					}
					if info.ModTime().After(cutoff) {
						continue
					}
					size := dirSize(full)
					if dry {
						total += size
						removed++
						if len(samples) < 20 {
							samples = append(samples, fmt.Sprintf("%s (%s)", full, fmtBytes(uint64(size))))
						}
						continue
					}
					if rerr := os.RemoveAll(full); rerr != nil {
						failures++
						fmt.Fprintf(&details, "删除失败 %s：%v\n", full, rerr)
						continue
					}
					total += size
					removed++
					if len(samples) < 20 {
						samples = append(samples, fmt.Sprintf("%s (%s)", full, fmtBytes(uint64(size))))
					}
				}
			}
			verb := "已删除"
			if dry {
				verb = "可删除"
			}
			var b strings.Builder
			fmt.Fprintf(&b, "%s %d 个超过 %d 天的临时项，共释放约 %s", verb, removed, days, fmtBytes(uint64(total)))
			if failures > 0 {
				fmt.Fprintf(&b, "（%d 项删除失败）", failures)
			}
			b.WriteString("。")
			if len(samples) > 0 {
				b.WriteString("\n示例：\n" + strings.Join(samples, "\n"))
			}
			if details.Len() > 0 {
				b.WriteString("\n" + details.String())
			}
			return aiagent.Result{OK: failures == 0, Summary: b.String(), Detail: b.String()}, nil
		},
	}
}

// cleanLogsTool truncates or rotates an oversized log file.
func cleanLogsTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "clean_logs",
		Title:       "清理日志",
		Description: "截断（清空）指定的日志文件以释放空间，执行前会记录原文件大小。仅允许清理系统已知的日志文件路径。",
		Risk:        aiagent.RiskLowWrite,
		Perm:        "file.write",
		Schema: schema(map[string]any{
			"path": pStr("日志文件的绝对路径，例如 /var/log/syslog"),
		}, "path"),
		Preview: func(args map[string]any) string {
			return "截断日志文件（内容将被清空）：" + str(args, "path")
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			path := str(args, "path")
			if path == "" {
				return failed("必须提供 path 参数"), nil
			}
			if !filepath.IsAbs(path) {
				return failed("必须使用绝对路径"), nil
			}
			if !allowedLogPath(d, path) {
				return failed("%s 不在可清理的日志白名单内。可用路径：%s", path, strings.Join(allowedLogPaths(d), "、")), nil
			}
			info, err := os.Stat(path)
			if err != nil {
				return failed("文件不存在或不可访问：%v", err), nil
			}
			if info.IsDir() {
				return failed("%s 是目录", path), nil
			}
			before := info.Size()
			if err := os.Truncate(path, 0); err != nil {
				return failed("截断 %s 失败：%v", path, err), nil
			}
			return textResult(fmt.Sprintf("已清空日志 %s，释放 %s。", path, fmtBytes(uint64(before)))), nil
		},
	}
}

// allowedLogPaths returns the whitelist of log files the agent may truncate.
func allowedLogPaths(d Deps) []string {
	if d.Log == nil {
		return nil
	}
	out := make([]string, 0, 16)
	for _, s := range d.Log.List() {
		out = append(out, s.Path)
	}
	return out
}

// allowedLogPath reports whether path is a known log file.
func allowedLogPath(d Deps, path string) bool {
	for _, p := range allowedLogPaths(d) {
		if p == path {
			return true
		}
	}
	return false
}

// serviceControlTool starts, stops or restarts a systemd service.
func serviceControlTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "service_control",
		Title:       "服务启停",
		Description: "启停或重启 systemd 服务（start/stop/restart/reload）。用于恢复异常服务或应用配置变更。",
		Risk:        aiagent.RiskLowWrite,
		Schema: schema(map[string]any{
			"service": pStr("服务名，例如 nginx、docker、opsmini"),
			"action":  pEnum("操作", "start", "stop", "restart", "reload"),
		}, "service", "action"),
		Preview: func(args map[string]any) string {
			return fmt.Sprintf("%s systemd 服务 %s", actionVerb(str(args, "action")), str(args, "service"))
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			name := str(args, "service")
			action := str(args, "action")
			if name == "" || action == "" {
				return failed("必须提供 service 与 action 参数"), nil
			}
			switch action {
			case "start", "stop", "restart", "reload":
			default:
				return failed("不支持的 action：%s", action), nil
			}
			// Never let the agent stop the panel itself.
			if action == "stop" && (name == "opsmini" || name == "opsmini.service") {
				return failed("禁止通过 AI 停止 OpsMini 面板自身服务"), nil
			}
			out, err := runCmd(ctx, 90*time.Second, "systemctl", action, name)
			summary := fmt.Sprintf("systemctl %s %s 执行%s。\n%s", action, name, okWord(err), truncate(strings.TrimSpace(out), 2000))
			if err != nil {
				return aiagent.Result{OK: false, Summary: summary}, nil
			}
			// Verify the resulting state.
			state, _ := runCmd(ctx, 10*time.Second, "systemctl", "is-active", name)
			summary += "\n当前状态：" + strings.TrimSpace(state)
			return textResult(summary), nil
		},
	}
}

// dockerControlTool starts, stops or restarts a container.
func dockerControlTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "docker_control",
		Title:       "容器启停",
		Description: "启动、停止或重启指定的 Docker 容器。",
		Risk:        aiagent.RiskLowWrite,
		Perm:        "container.edit",
		Schema: schema(map[string]any{
			"container": pStr("容器名称或 ID"),
			"action":    pEnum("操作", "start", "stop", "restart"),
		}, "container", "action"),
		Preview: func(args map[string]any) string {
			return fmt.Sprintf("%s 容器 %s", actionVerb(str(args, "action")), str(args, "container"))
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			name := str(args, "container")
			action := str(args, "action")
			if name == "" || action == "" {
				return failed("必须提供 container 与 action 参数"), nil
			}
			var err error
			switch action {
			case "start":
				err = d.Docker.StartContainer(name)
			case "stop":
				err = d.Docker.StopContainer(name)
			case "restart":
				err = d.Docker.RestartContainer(name)
			default:
				return failed("不支持的 action：%s", action), nil
			}
			if err != nil {
				return failed("容器 %s %s 失败：%v", name, action, err), nil
			}
			return textResult(fmt.Sprintf("容器 %s 已执行 %s。", name, action)), nil
		},
	}
}

// installSkillTool installs a skill from SkillHub.
func installSkillTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "install_skill",
		Title:       "安装技能",
		Description: "从 SkillHub 市场下载并安装一个技能（Skill）。安装前建议先用 search_skills 确认 slug。",
		Risk:        aiagent.RiskLowWrite,
		Perm:        "skill.manage",
		Schema: schema(map[string]any{
			"slug": pStr("技能的 slug 标识"),
		}, "slug"),
		Preview: func(args map[string]any) string {
			return "从 SkillHub 安装技能：" + str(args, "slug")
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			slug := str(args, "slug")
			if slug == "" {
				return failed("必须提供 slug 参数"), nil
			}
			if err := d.Skill.Install(slug); err != nil {
				return failed("安装技能 %s 失败：%v", slug, err), nil
			}
			return textResult(fmt.Sprintf("技能 %s 安装成功。", slug)), nil
		},
	}
}

// --- L2 ---

// execCommandTool runs an arbitrary shell command.
func execCommandTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "exec_command",
		Title:       "执行命令",
		Description: "在服务器上执行一条 Shell 命令并返回输出。这是唯一可以执行任意命令的工具，请在确认只读工具无法定位问题时才使用，并优先使用幂等的查询命令。",
		Risk:        aiagent.RiskHighWrite,
		Perm:        "ai.agent.write",
		Schema: schema(map[string]any{
			"command": pStr("要执行的 Shell 命令"),
			"workdir": pStr("工作目录（可选）"),
			"timeout": pInt("超时秒数，默认 60，最大 600"),
		}, "command"),
		Preview: func(args map[string]any) string {
			return "在服务器上执行命令：`" + str(args, "command") + "`"
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			command := str(args, "command")
			if command == "" {
				return failed("必须提供 command 参数"), nil
			}
			timeout := intArg(args, "timeout", 60)
			if timeout <= 0 || timeout > 600 {
				timeout = 60
			}
			cctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
			defer cancel()

			cmd := exec.CommandContext(cctx, "sh", "-c", command)
			if wd := str(args, "workdir"); wd != "" && filepath.IsAbs(wd) {
				cmd.Dir = wd
			}
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			start := time.Now()
			runErr := cmd.Run()
			elapsed := time.Since(start)

			out := truncate(strings.TrimSpace(stdout.String()), maxExecOutput)
			errOut := truncate(strings.TrimSpace(stderr.String()), 16<<10)
			exitCode := 0
			if runErr != nil {
				var ee *exec.ExitError
				if errors.As(runErr, &ee) {
					exitCode = ee.ExitCode()
				} else {
					exitCode = -1
					errOut += "\n" + runErr.Error()
				}
			}
			timedOut := errors.Is(cctx.Err(), context.DeadlineExceeded)

			var b strings.Builder
			fmt.Fprintf(&b, "$ %s\n", command)
			fmt.Fprintf(&b, "退出码：%d，耗时 %s", exitCode, elapsed.Round(time.Millisecond))
			if timedOut {
				fmt.Fprintf(&b, "（超时 %ds 被终止）", timeout)
			}
			b.WriteString("\n")
			if out != "" {
				b.WriteString("--- stdout ---\n" + out + "\n")
			}
			if errOut != "" {
				b.WriteString("--- stderr ---\n" + errOut + "\n")
			}
			ok := runErr == nil && !timedOut
			return aiagent.Result{OK: ok, Summary: b.String(), Detail: b.String()}, nil
		},
	}
}

// writeFileTool writes content to a file.
func writeFileTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "write_file",
		Title:       "写入文件",
		Description: "把内容写入服务器上的文件（覆盖或追加）。写入前会自动备份已存在的文件，便于回滚。",
		Risk:        aiagent.RiskHighWrite,
		Perm:        "ai.agent.write",
		Schema: schema(map[string]any{
			"path":    pStr("文件绝对路径"),
			"content": pStr("要写入的完整内容"),
			"append":  pBool("true 表示追加而不是覆盖，默认 false"),
			"mode":    pStr("文件权限，八进制，例如 0644（可选）"),
		}, "path", "content"),
		Preview: func(args map[string]any) string {
			verb := "覆盖写入"
			if boolArg(args, "append", false) {
				verb = "追加写入"
			}
			return fmt.Sprintf("%s文件 %s（%d 字节）", verb, str(args, "path"), len(str(args, "content")))
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			path := str(args, "path")
			if path == "" {
				return failed("必须提供 path 参数"), nil
			}
			if !filepath.IsAbs(path) {
				return failed("必须使用绝对路径"), nil
			}
			content := str(args, "content")
			appendMode := boolArg(args, "append", false)

			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return failed("创建目录失败：%v", err), nil
			}
			backup := ""
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				backup = path + ".opsmini-bak-" + time.Now().Format("20060102150405")
				if data, rerr := os.ReadFile(path); rerr == nil {
					if werr := os.WriteFile(backup, data, info.Mode()); werr != nil {
						backup = ""
					}
				}
			}
			flag := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
			if appendMode {
				flag = os.O_CREATE | os.O_WRONLY | os.O_APPEND
			}
			f, err := os.OpenFile(path, flag, 0o644)
			if err != nil {
				return failed("打开文件失败：%v", err), nil
			}
			if _, err := f.WriteString(content); err != nil {
				f.Close()
				return failed("写入失败：%v", err), nil
			}
			f.Close()
			if m := str(args, "mode"); m != "" {
				if perm, perr := parseOctal(m); perr == nil {
					_ = os.Chmod(path, perm)
				}
			}
			msg := fmt.Sprintf("已写入 %s（%d 字节）", path, len(content))
			if backup != "" {
				msg += "，原文件已备份为 " + backup
			}
			return textResult(msg), nil
		},
	}
}

// deletePathTool deletes a file or directory.
func deletePathTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "delete_path",
		Title:       "删除文件",
		Description: "删除服务器上的文件或目录（目录会递归删除）。执行前会自动统计待删除内容的大小。",
		Risk:        aiagent.RiskHighWrite,
		Perm:        "ai.agent.write",
		Schema: schema(map[string]any{
			"path": pStr("要删除的文件或目录的绝对路径"),
		}, "path"),
		Preview: func(args map[string]any) string {
			path := str(args, "path")
			info, err := os.Stat(path)
			if err == nil && info.IsDir() {
				return fmt.Sprintf("递归删除目录 %s（当前大小 %s）", path, fmtBytes(uint64(dirSize(path))))
			}
			return "删除文件/路径：" + path
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			path := str(args, "path")
			if path == "" {
				return failed("必须提供 path 参数"), nil
			}
			if !filepath.IsAbs(path) {
				return failed("必须使用绝对路径"), nil
			}
			if path == "/" || path == "/etc" || path == "/usr" || path == "/var" || path == "/home" {
				return failed("拒绝删除系统关键目录 %s", path), nil
			}
			info, err := os.Stat(path)
			if err != nil {
				return failed("路径不存在或不可访问：%v", err), nil
			}
			size := info.Size()
			if info.IsDir() {
				size = dirSize(path)
			}
			if err := os.RemoveAll(path); err != nil {
				return failed("删除 %s 失败：%v", path, err), nil
			}
			return textResult(fmt.Sprintf("已删除 %s，释放 %s。", path, fmtBytes(uint64(size)))), nil
		},
	}
}

// modifySSHDTool changes a key in /etc/ssh/sshd_config.
func modifySSHDTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "modify_sshd",
		Title:       "修改 SSH 配置",
		Description: "修改 /etc/ssh/sshd_config 中的配置项（如 PermitRootLogin、PasswordAuthentication、Port），修改前自动备份，修改后执行 sshd -t 语法校验。",
		Risk:        aiagent.RiskHighWrite,
		Perm:        "ai.agent.write",
		Schema: schema(map[string]any{
			"key":    pStr("配置项名称，例如 PermitRootLogin"),
			"value":  pStr("配置值，例如 no"),
			"reload": pBool("修改后是否重载 sshd 使配置生效，默认 false"),
		}, "key", "value"),
		Preview: func(args map[string]any) string {
			return fmt.Sprintf("修改 /etc/ssh/sshd_config：设置 %s = %s", str(args, "key"), str(args, "value"))
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			key := str(args, "key")
			value := str(args, "value")
			if key == "" {
				return failed("必须提供 key 参数"), nil
			}
			if !regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]{0,40}$`).MatchString(key) {
				return failed("非法的配置项名称：%s", key), nil
			}
			const sshdConfig = "/etc/ssh/sshd_config"
			data, err := os.ReadFile(sshdConfig)
			if err != nil {
				return failed("读取 %s 失败：%v", sshdConfig, err), nil
			}
			backup := sshdConfig + ".opsmini-bak-" + time.Now().Format("20060102150405")
			if err := os.WriteFile(backup, data, 0o600); err != nil {
				return failed("备份失败：%v", err), nil
			}
			lines := strings.Split(string(data), "\n")
			keyRe := regexp.MustCompile(`(?i)^\s*#?\s*` + key + `\s`)
			replaced := false
			for i, line := range lines {
				if keyRe.MatchString(line) {
					if !replaced {
						lines[i] = key + " " + value
						replaced = true
					} else {
						lines[i] = "#" + strings.TrimLeft(line, "#")
					}
				}
			}
			if !replaced {
				lines = append(lines, key+" "+value)
			}
			out := strings.Join(lines, "\n")
			if err := os.WriteFile(sshdConfig, []byte(out), 0o600); err != nil {
				_ = os.WriteFile(sshdConfig, data, 0o600)
				return failed("写入失败，已回滚：%v", err), nil
			}
			// Validate syntax; roll back when the config is broken.
			if check, cerr := runCmd(ctx, 15*time.Second, "sshd", "-t"); cerr != nil {
				_ = os.WriteFile(sshdConfig, data, 0o600)
				return failed("sshd 配置校验失败，已回滚：%s", truncate(strings.TrimSpace(check), 500)), nil
			}
			msg := fmt.Sprintf("已设置 %s = %s，原配置备份为 %s。", key, value, backup)
			if boolArg(args, "reload", false) {
				if out2, rerr := runCmd(ctx, 30*time.Second, "systemctl", "reload", "sshd"); rerr != nil {
					if out3, rerr3 := runCmd(ctx, 30*time.Second, "systemctl", "reload", "ssh"); rerr3 != nil {
						msg += fmt.Sprintf("\n重载 sshd 失败：%s / %s", truncate(out2, 200), truncate(out3, 200))
					} else {
						msg += "\n已重载 ssh 服务，配置生效。"
					}
				} else {
					msg += "\n已重载 sshd 服务，配置生效。"
				}
			} else {
				msg += "\n注意：配置尚未生效，需要重载 sshd 或重启服务。"
			}
			return textResult(msg), nil
		},
	}
}

// compressDirTool archives a directory into a zip file.
func compressDirTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "compress_dir",
		Title:       "压缩目录",
		Description: "把指定目录压缩为 zip 包（默认存放到 /tmp），用于备份或归档。",
		Risk:        aiagent.RiskHighWrite,
		Perm:        "ai.agent.write",
		Schema: schema(map[string]any{
			"path": pStr("要压缩的目录绝对路径"),
		}, "path"),
		Preview: func(args map[string]any) string {
			return "压缩目录：" + str(args, "path")
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			path := str(args, "path")
			if path == "" {
				return failed("必须提供 path 参数"), nil
			}
			tmpPath, name, err := d.File.CompressDir(path)
			if err != nil {
				return failed("压缩 %s 失败：%v", path, err), nil
			}
			info, _ := os.Stat(tmpPath)
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			return textResult(fmt.Sprintf("已压缩 %s 为 %s（%s，大小 %s）。可在面板文件管理页下载或继续移动。",
				path, filepath.Base(tmpPath), name, fmtBytes(uint64(size)))), nil
		},
	}
}

// cronWriteTool creates or updates a panel scheduled task.
func cronWriteTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "cron_write",
		Title:       "创建计划任务",
		Description: "在 OpsMini 面板中创建或更新一条计划任务（crontab 表达式 + 命令）。任务由面板持久化并调度执行。",
		Risk:        aiagent.RiskHighWrite,
		Perm:        "ai.agent.write",
		Schema: schema(map[string]any{
			"name":     pStr("任务名称"),
			"schedule": pStr("标准 5 段 cron 表达式，例如 */5 * * * *"),
			"command":  pStr("要执行的命令"),
			"enabled":  pBool("是否启用，默认 true"),
		}, "name", "schedule", "command"),
		Preview: func(args map[string]any) string {
			return fmt.Sprintf("创建计划任务「%s」：%s 执行 `%s`", str(args, "name"), str(args, "schedule"), str(args, "command"))
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			if d.Cron == nil {
				return failed("计划任务服务不可用"), nil
			}
			name, sched, cmd := str(args, "name"), str(args, "schedule"), str(args, "command")
			if name == "" || sched == "" || cmd == "" {
				return failed("name、schedule、command 均为必填"), nil
			}
			enabled := boolArg(args, "enabled", true)
			job, err := d.Cron.Create(service.CronReq{
				Name: name, Kind: model.CronKindPanel, Schedule: sched, Command: cmd, Enabled: &enabled,
			})
			if err != nil {
				return failed("创建计划任务失败：%v", err), nil
			}
			return textResult(fmt.Sprintf("已创建计划任务 #%d：%s（%s）%s", job.ID, job.Name, job.Schedule, job.Command)), nil
		},
	}
}

// killProcessTool sends a signal to a process.
func killProcessTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "kill_process",
		Title:       "结束进程",
		Description: "向指定进程发送信号（默认 SIGTERM，可指定 SIGKILL）。用于终止异常占用资源的进程。",
		Risk:        aiagent.RiskHighWrite,
		Perm:        "ai.agent.write",
		Schema: schema(map[string]any{
			"pid":    pInt("进程 PID"),
			"signal": pEnum("信号", "term", "kill"),
		}, "pid"),
		Preview: func(args map[string]any) string {
			return fmt.Sprintf("向进程 PID %d 发送 SIG%s", intArg(args, "pid", 0), strings.ToUpper(strDef(args, "signal", "term")))
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			pid := intArg(args, "pid", 0)
			if pid <= 0 {
				return failed("必须提供有效的 pid 参数"), nil
			}
			if pid == 1 {
				return failed("拒绝向 1 号进程发送信号"), nil
			}
			sig := syscallTerm
			if strings.EqualFold(str(args, "signal"), "kill") {
				sig = syscallKill
			}
			if err := d.Sys.KillProcess(int32(pid), sig); err != nil {
				return failed("结束进程 %d 失败：%v", pid, err), nil
			}
			return textResult(fmt.Sprintf("已向进程 %d 发送信号 %v。", pid, sig)), nil
		},
	}
}

// firewallRuleTool manages firewall ports.
func firewallRuleTool(d Deps) aiagent.Tool {
	return aiagent.Tool{
		Name:        "firewall_rule",
		Title:       "防火墙规则",
		Description: "开放/关闭端口，或启用/停用防火墙。关闭面板自身端口会被安全策略拒绝。",
		Risk:        aiagent.RiskHighWrite,
		Perm:        "ai.agent.write",
		Schema: schema(map[string]any{
			"action": pEnum("操作", "allow_port", "deny_port", "enable", "disable"),
			"port":   pInt("端口号（allow_port / deny_port 时必填）"),
			"proto":  pEnum("协议", "tcp", "udp"),
		}, "action"),
		Preview: func(args map[string]any) string {
			switch str(args, "action") {
			case "enable":
				return "启用防火墙"
			case "disable":
				return "停用防火墙（会放开所有端口，慎重）"
			case "deny_port":
				return fmt.Sprintf("关闭端口 %d/%s", intArg(args, "port", 0), strDef(args, "proto", "tcp"))
			default:
				return fmt.Sprintf("开放端口 %d/%s", intArg(args, "port", 0), strDef(args, "proto", "tcp"))
			}
		},
		Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
			action := str(args, "action")
			proto := strDef(args, "proto", "tcp")
			port := intArg(args, "port", 0)
			switch action {
			case "enable":
				if err := d.Firewall.Enable(); err != nil {
					return failed("启用防火墙失败：%v", err), nil
				}
				return textResult("防火墙已启用。"), nil
			case "disable":
				if err := d.Firewall.Disable(); err != nil {
					return failed("停用防火墙失败：%v", err), nil
				}
				return textResult("防火墙已停用。"), nil
			case "allow_port", "deny_port":
				if port <= 0 || port > 65535 {
					return failed("端口号无效"), nil
				}
				if action == "allow_port" {
					if err := d.Firewall.AllowPort(port, proto); err != nil {
						return failed("开放端口失败：%v", err), nil
					}
					return textResult(fmt.Sprintf("已开放 %d/%s。", port, proto)), nil
				}
				if err := d.Firewall.DenyPort(port, proto); err != nil {
					return failed("关闭端口失败：%v", err), nil
				}
				return textResult(fmt.Sprintf("已关闭 %d/%s。", port, proto)), nil
			}
			return failed("不支持的 action：%s", action), nil
		},
	}
}

// --- shared helpers ---

// dirSize returns the recursive size of a path.
func dirSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	if !info.IsDir() {
		return info.Size()
	}
	var total int64
	_ = filepath.WalkDir(path, func(_ string, e fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if fi, ierr := e.Info(); ierr == nil && !e.IsDir() {
			total += fi.Size()
		}
		return nil
	})
	return total
}

// actionVerb renders an action as a Chinese verb for the confirmation dialog.
func actionVerb(action string) string {
	switch action {
	case "start":
		return "启动"
	case "stop":
		return "停止"
	case "restart":
		return "重启"
	case "reload":
		return "重载"
	}
	return action
}

// okWord renders a success word.
func okWord(err error) string {
	if err == nil {
		return "成功"
	}
	return "失败"
}

// parseOctal parses an octal file mode string.
func parseOctal(s string) (os.FileMode, error) {
	var v uint32
	if _, err := fmt.Sscanf(strings.TrimPrefix(s, "0o"), "%o", &v); err != nil {
		return 0, err
	}
	return os.FileMode(v), nil
}

// sortStrings sorts a copy of the input slice.
func sortStrings(in []string) []string {
	out := append([]string{}, in...)
	sort.Strings(out)
	return out
}
