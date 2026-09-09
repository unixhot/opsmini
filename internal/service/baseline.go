package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// BaselineService performs security baseline checks (one-click health check).
type BaselineService struct {
	repo *repository.BaselineResultRepo
	sys  *SystemService
}

// NewBaselineService creates a BaselineService.
func NewBaselineService(repo *repository.BaselineResultRepo, sys *SystemService) *BaselineService {
	return &BaselineService{repo: repo, sys: sys}
}

type checkItem struct {
	key, category, title string
	weight               int
	run                  func() (status, detail, suggestion string)
}

// RunScan runs all check items and persists them, returning this batch's results.
func (s *BaselineService) RunScan() ([]model.BaselineResult, error) {
	scanID := strconv.FormatInt(time.Now().UnixNano(), 10)
	results := make([]model.BaselineResult, 0, len(s.items()))
	for _, it := range s.items() {
		status, detail, sug := it.run()
		r := model.BaselineResult{
			ScanID: scanID, Category: it.category, ItemKey: it.key,
			Title: it.title, Status: status, Detail: detail, Suggestion: sug, Weight: it.weight,
		}
		_ = s.repo.Create(&r)
		results = append(results, r)
	}
	return results, nil
}

// Latest returns the most recent health check results (empty slice if none).
func (s *BaselineService) Latest() ([]model.BaselineResult, error) {
	scanID, err := s.repo.LatestScanID()
	if err != nil {
		return []model.BaselineResult{}, nil
	}
	return s.repo.ListByScanID(scanID)
}

// items defines all check items (built into the code).
func (s *BaselineService) items() []checkItem {
	return []checkItem{
		{key: "ssh_root_login", category: "ssh", title: "SSH 禁止 root 密码登录", weight: 3, run: func() (string, string, string) {
			v := sshdValue("PermitRootLogin")
			if v == "" {
				return "warn", "未显式配置 PermitRootLogin（默认可能允许）", "建议在 /etc/ssh/sshd_config 中设置 PermitRootLogin no"
			}
			if strings.EqualFold(v, "no") || strings.EqualFold(v, "prohibit-password") {
				return "pass", "PermitRootLogin = " + v, ""
			}
			return "fail", "PermitRootLogin = " + v, "建议设为 no 或 prohibit-password，改用普通用户 + sudo"
		}},
		{key: "ssh_password_auth", category: "ssh", title: "SSH 关闭密码登录（改用密钥）", weight: 3, run: func() (string, string, string) {
			v := sshdValue("PasswordAuthentication")
			if strings.EqualFold(v, "no") {
				return "pass", "PasswordAuthentication = no", ""
			}
			return "warn", "PasswordAuthentication 未关闭", "建议密钥登录验证通过后设为 PasswordAuthentication no"
		}},
		{key: "ssh_port", category: "ssh", title: "SSH 修改默认端口 22", weight: 1, run: func() (string, string, string) {
			v := sshdValue("Port")
			if v != "" && v != "22" {
				return "pass", "SSH 端口 = " + v, ""
			}
			return "warn", "SSH 使用默认端口 22", "建议修改为非常用端口以降低扫描风险"
		}},
		{key: "ssh_protocol", category: "ssh", title: "SSH 协议版本为 2", weight: 1, run: func() (string, string, string) {
			v := sshdValue("Protocol")
			if v == "" || v == "2" {
				return "pass", "Protocol = 2（默认）", ""
			}
			return "fail", "Protocol = " + v, "应强制使用 Protocol 2"
		}},
		{key: "shadow_empty_pass", category: "password", title: "无空密码账户", weight: 3, run: func() (string, string, string) {
			names := shadowEmptyPasswordAccounts()
			if len(names) == 0 {
				return "pass", "未发现空密码账户", ""
			}
			return "fail", "存在空密码账户：" + strings.Join(names, ", "), "为空密码账户设置密码或锁定（passwd -l）"
		}},
		{key: "pass_max_days", category: "password", title: "密码最大有效期 ≤ 90 天", weight: 2, run: func() (string, string, string) {
			v := loginDefsValue("PASS_MAX_DAYS")
			n, _ := strconv.Atoi(v)
			if v != "" && n > 0 && n <= 90 {
				return "pass", "PASS_MAX_DAYS = " + v, ""
			}
			return "warn", "PASS_MAX_DAYS = " + baselineOrElse(v, "未设置"), "建议在 /etc/login.defs 设置 PASS_MAX_DAYS 90"
		}},
		{key: "uid0_account", category: "account", title: "无多余 UID=0 账户", weight: 3, run: func() (string, string, string) {
			names := uidZeroAccounts()
			if len(names) == 0 {
				return "pass", "仅 root 拥有 UID=0", ""
			}
			return "fail", "存在 UID=0 的非 root 账户：" + strings.Join(names, ", "), "检查这些账户是否必要，删除或改 UID"
		}},
		{key: "login_no_pass", category: "account", title: "无可登录的无密码账户", weight: 2, run: func() (string, string, string) {
			names := loginNoPasswordAccounts()
			if len(names) == 0 {
				return "pass", "未发现可登录且无密码的账户", ""
			}
			return "fail", "可登录且无密码账户：" + strings.Join(names, ", "), "设置密码或锁定这些账户"
		}},
		{key: "shadow_perm", category: "file_perm", title: "/etc/shadow 权限安全", weight: 2, run: func() (string, string, string) {
			p, ok := filePerm("/etc/shadow")
			if !ok {
				return "warn", "无法读取 /etc/shadow 权限", "检查文件是否存在"
			}
			if p <= 0o600 {
				return "pass", fmt.Sprintf("权限 %04o", p), ""
			}
			return "fail", fmt.Sprintf("权限 %04o（过宽）", p), "执行 chmod 600 /etc/shadow"
		}},
		{key: "passwd_perm", category: "file_perm", title: "/etc/passwd 权限安全", weight: 2, run: func() (string, string, string) {
			p, ok := filePerm("/etc/passwd")
			if !ok {
				return "warn", "无法读取 /etc/passwd 权限", "检查文件是否存在"
			}
			if p <= 0o644 {
				return "pass", fmt.Sprintf("权限 %04o", p), ""
			}
			return "fail", fmt.Sprintf("权限 %04o（过宽）", p), "执行 chmod 644 /etc/passwd"
		}},
		{key: "sshd_perm", category: "file_perm", title: "/etc/ssh/sshd_config 权限安全", weight: 1, run: func() (string, string, string) {
			p, ok := filePerm("/etc/ssh/sshd_config")
			if !ok {
				return "pass", "未找到 sshd_config", ""
			}
			if p <= 0o600 {
				return "pass", fmt.Sprintf("权限 %04o", p), ""
			}
			return "warn", fmt.Sprintf("权限 %04o", p), "建议 chmod 600 /etc/ssh/sshd_config"
		}},
		{key: "firewall_enabled", category: "firewall", title: "防火墙已启用", weight: 3, run: func() (string, string, string) {
			fw := s.sys.FirewallStatus()
			if fw.Enabled {
				return "pass", "防火墙已启用（" + fw.Backend + "）", ""
			}
			return "fail", "未检测到启用的防火墙", "启用 ufw / firewalld，或在安全页配置防火墙规则"
		}},
		{key: "highrisk_port", category: "firewall", title: "无高危端口监听", weight: 3, run: func() (string, string, string) {
			risky := map[int]string{23: "Telnet", 445: "SMB", 135: "RPC", 1433: "MSSQL", 3389: "RDP"}
			found := []string{}
			ports, _ := s.sys.Ports()
			for _, p := range ports {
				if name, ok := risky[int(p.Port)]; ok {
					found = append(found, fmt.Sprintf("%d(%s)", p.Port, name))
				}
			}
			if len(found) == 0 {
				return "pass", "未发现高危端口监听", ""
			}
			return "warn", "高危端口监听：" + strings.Join(found, ", "), "确认是否必要，否则关闭或限制来源 IP"
		}},
		{key: "cron_suspicious", category: "cron", title: "无可疑计划任务", weight: 3, run: func() (string, string, string) {
			sus := suspiciousCronJobs()
			if len(sus) == 0 {
				return "pass", "未发现可疑计划任务", ""
			}
			return "warn", "发现可疑计划任务：" + strings.Join(sus, "; "), "检查计划任务，删除不明来源的下载执行命令"
		}},
	}
}

// ---- the following are helper functions (file parsing) ----

// readFileTrim reads a file and returns its trimmed content, or "" on any error.
func readFileTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// sshdValue parses the value of a directive in sshd_config (including sshd_config.d/*.conf).
func sshdValue(key string) string {
	files := []string{"/etc/ssh/sshd_config"}
	if entries, err := filepath.Glob("/etc/ssh/sshd_config.d/*.conf"); err == nil {
		files = append(files, entries...)
	}
	for _, f := range files {
		for _, line := range strings.Split(readFileTrim(f), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.Fields(line)
			if len(parts) >= 2 && strings.EqualFold(parts[0], key) {
				return parts[1]
			}
		}
	}
	return ""
}

// loginDefsValue parses a value in /etc/login.defs.
func loginDefsValue(key string) string {
	for _, line := range strings.Split(readFileTrim("/etc/login.defs"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) >= 2 && parts[0] == key {
			return parts[1]
		}
	}
	return ""
}

// baselineOrElse returns v when non-empty, otherwise def.
func baselineOrElse(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// filePerm returns the permission bits of a file, or ok=false if it cannot be stat'd.
func filePerm(path string) (uint32, bool) {
	fi, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	return uint32(fi.Mode().Perm()), true
}

// shadowEmptyPasswordAccounts returns accounts with an empty password field.
func shadowEmptyPasswordAccounts() []string {
	var out []string
	for _, line := range strings.Split(readFileTrim("/etc/shadow"), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) >= 2 && fields[1] == "" {
			out = append(out, fields[0])
		}
	}
	return out
}

// uidZeroAccounts returns non-root accounts with UID=0.
func uidZeroAccounts() []string {
	var out []string
	for _, line := range strings.Split(readFileTrim("/etc/passwd"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) >= 3 && fields[2] == "0" && fields[0] != "root" {
			out = append(out, fields[0])
		}
	}
	return out
}

// loginNoPasswordAccounts returns accounts that are loggable and passwordless (shadow locked/empty).
func loginNoPasswordAccounts() []string {
	shells := map[string]bool{"/bin/bash": true, "/bin/sh": true, "/bin/zsh": true, "/usr/bin/bash": true}
	noPass := map[string]bool{}
	for _, line := range strings.Split(readFileTrim("/etc/shadow"), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) >= 2 && (fields[1] == "" || strings.HasPrefix(fields[1], "!")) {
			noPass[fields[0]] = true
		}
	}
	var out []string
	for _, line := range strings.Split(readFileTrim("/etc/passwd"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) >= 7 && shells[fields[6]] && noPass[fields[0]] {
			out = append(out, fields[0])
		}
	}
	return out
}

// suspiciousCronJobs detects suspicious download-execute commands in crontab.
func suspiciousCronJobs() []string {
	cronSvc := NewCrontabService()
	all := append(cronSvc.SystemJobs(), cronSvc.UserJobs()...)
	patterns := []string{"curl", "wget", "base64 -d", "/tmp/", "/dev/shm"}
	var out []string
	for _, job := range all {
		cmd := strings.ToLower(job.Command)
		for _, p := range patterns {
			if strings.Contains(cmd, p) {
				out = append(out, job.Command)
				break
			}
		}
	}
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// systemctlEnabledServices returns the list of systemd enabled services (helper).
func systemctlEnabledServices() []string {
	out, err := exec.Command("systemctl", "list-unit-files", "--type=service", "--state=enabled", "--no-pager", "--no-legend").Output()
	if err != nil {
		return nil
	}
	var res []string
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) >= 1 {
			res = append(res, f[0])
		}
	}
	return res
}
