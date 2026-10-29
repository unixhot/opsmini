package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// extraItems is the v1.4 baseline expansion: the CIS-flavoured checks that were
// missing from the original 13. They are kept in a separate file so the original
// list stays readable, but they behave identically (same scoring formula).
//
// Every item must be answerable from local files only: no network calls, no
// external scanners. A baseline that needs an agent on the other side would
// belong in OpsAnt, not here.
func (s *BaselineService) extraItems() []checkItem {
	return []checkItem{
		// --- 账号与认证 ---------------------------------------------------
		{key: "uid0_non_root", category: "account", title: "除 root 外没有 UID 0 的账号", weight: 3, run: func() (string, string, string) {
			var bad []string
			for _, line := range passwdLines() {
				f := strings.Split(line, ":")
				if len(f) >= 3 && f[2] == "0" && f[0] != "root" {
					bad = append(bad, f[0])
				}
			}
			if len(bad) == 0 {
				return "pass", "只有 root 的 UID 为 0", ""
			}
			return "fail", "存在 UID 0 的普通账号：" + strings.Join(bad, ", "), "立即修改这些账号的 UID，UID 0 等同于 root"
		}},
		{key: "pass_min_len", category: "password", title: "密码最短长度 ≥ 8", weight: 2, run: func() (string, string, string) {
			v := loginDefsValue("PASS_MIN_LEN")
			n, err := strconv.Atoi(v)
			if err != nil {
				return "warn", "未配置 PASS_MIN_LEN", "在 /etc/login.defs 设置 PASS_MIN_LEN 8"
			}
			if n >= 8 {
				return "pass", fmt.Sprintf("PASS_MIN_LEN = %d", n), ""
			}
			return "fail", fmt.Sprintf("PASS_MIN_LEN = %d", n), "建议 ≥ 8，配合 pam_pwquality 使用"
		}},
		{key: "pass_min_days", category: "password", title: "密码最短使用期 ≥ 1 天", weight: 1, run: func() (string, string, string) {
			n := atoiOrZero(loginDefsValue("PASS_MIN_DAYS"))
			if n >= 1 {
				return "pass", fmt.Sprintf("PASS_MIN_DAYS = %d", n), ""
			}
			return "warn", "PASS_MIN_DAYS 未设置或为 0", "建议设为 1，防止立即改回旧密码"
		}},
		{key: "pass_warn_age", category: "password", title: "密码到期前 7 天提醒", weight: 1, run: func() (string, string, string) {
			n := atoiOrZero(loginDefsValue("PASS_WARN_AGE"))
			if n >= 7 {
				return "pass", fmt.Sprintf("PASS_WARN_AGE = %d", n), ""
			}
			return "warn", "PASS_WARN_AGE 小于 7", "建议设为 7 天，避免密码突然过期"
		}},
		{key: "sudo_nopasswd", category: "account", title: "sudo 未配置免密提权", weight: 2, run: func() (string, string, string) {
			files := []string{"/etc/sudoers"}
			if entries, err := os.ReadDir("/etc/sudoers.d"); err == nil {
				for _, e := range entries {
					files = append(files, "/etc/sudoers.d/"+e.Name())
				}
			}
			var hits []string
			for _, f := range files {
				data, err := os.ReadFile(f)
				if err != nil {
					continue
				}
				for _, line := range strings.Split(string(data), "\n") {
					trimmed := strings.TrimSpace(line)
					if trimmed == "" || strings.HasPrefix(trimmed, "#") {
						continue
					}
					if strings.Contains(trimmed, "NOPASSWD") {
						hits = append(hits, f)
						break
					}
				}
			}
			if len(hits) == 0 {
				return "pass", "未发现 NOPASSWD 配置", ""
			}
			return "warn", "存在免密 sudo：" + strings.Join(hits, ", "), "确认这些免密规则是必需的，否则移除"
		}},
		{key: "root_home_perm", category: "perm", title: "/root 目录权限 700", weight: 2, run: func() (string, string, string) {
			perm, ok := filePerm("/root")
			if !ok {
				return "warn", "无法读取 /root 权限", ""
			}
			if perm&0o077 == 0 {
				return "pass", fmt.Sprintf("/root 权限 %o", perm), ""
			}
			return "fail", fmt.Sprintf("/root 权限 %o", perm), "chmod 700 /root"
		}},
		{key: "system_accounts_login", category: "account", title: "系统账号不可交互登录", weight: 2, run: func() (string, string, string) {
			var bad []string
			for _, line := range passwdLines() {
				f := strings.Split(line, ":")
				if len(f) < 7 {
					continue
				}
				uid := atoiOrZero(f[2])
				shell := f[6]
				if uid > 0 && uid < 1000 && !strings.Contains(shell, "nologin") && !strings.Contains(shell, "false") && shell != "" {
					bad = append(bad, f[0])
				}
			}
			if len(bad) == 0 {
				return "pass", "系统账号均为 nologin/false", ""
			}
			return "warn", "可登录的系统账号：" + strings.Join(bad, ", "), "把不用的服务账号 shell 改为 /usr/sbin/nologin"
		}},

		// --- SSH 加固 -----------------------------------------------------
		{key: "ssh_max_auth_tries", category: "ssh", title: "SSH MaxAuthTries ≤ 4", weight: 2, run: sshdNumeric("MaxAuthTries", 4, false)},
		{key: "ssh_client_alive", category: "ssh", title: "SSH 空闲超时 ≤ 300 秒", weight: 1, run: sshdNumeric("ClientAliveInterval", 300, false)},
		{key: "ssh_login_grace", category: "ssh", title: "SSH 登录宽限 ≤ 60 秒", weight: 1, run: sshdNumeric("LoginGraceTime", 60, false)},
		{key: "ssh_permit_empty", category: "ssh", title: "SSH 禁止空密码登录", weight: 3, run: func() (string, string, string) {
			v := sshdValue("PermitEmptyPasswords")
			if strings.EqualFold(v, "yes") {
				return "fail", "PermitEmptyPasswords = yes", "改为 no"
			}
			return "pass", "PermitEmptyPasswords 未开启", ""
		}},
		{key: "ssh_x11_forward", category: "ssh", title: "SSH 关闭 X11 转发", weight: 1, run: func() (string, string, string) {
			v := sshdValue("X11Forwarding")
			if strings.EqualFold(v, "yes") {
				return "warn", "X11Forwarding = yes", "服务器一般不需要 X11，建议设为 no"
			}
			return "pass", "X11Forwarding 未开启", ""
		}},
		{key: "ssh_allow_users", category: "ssh", title: "SSH 配置了 AllowUsers/AllowGroups", weight: 1, run: func() (string, string, string) {
			if v := sshdValue("AllowUsers"); v != "" {
				return "pass", "AllowUsers = " + v, ""
			}
			if v := sshdValue("AllowGroups"); v != "" {
				return "pass", "AllowGroups = " + v, ""
			}
			return "warn", "未限制可登录用户", "建议用 AllowUsers 或 AllowGroups 收敛入口"
		}},

		// --- 文件权限 -----------------------------------------------------
		{key: "gshadow_perm", category: "perm", title: "/etc/gshadow 权限 ≤ 640", weight: 2, run: permAtMost("/etc/gshadow", 0o640)},
		{key: "group_perm", category: "perm", title: "/etc/group 权限 ≤ 644", weight: 1, run: permAtMost("/etc/group", 0o644)},
		{key: "crontab_perm", category: "perm", title: "/etc/crontab 权限 ≤ 600", weight: 2, run: permAtMost("/etc/crontab", 0o600)},
		{key: "boot_perm", category: "perm", title: "/boot 目录权限 ≤ 700", weight: 1, run: permAtMost("/boot", 0o700)},
		{key: "ssh_host_key_perm", category: "perm", title: "SSH 主机私钥权限 ≤ 600", weight: 3, run: func() (string, string, string) {
			matches, _ := filepathGlob("/etc/ssh/ssh_host_*_key")
			if len(matches) == 0 {
				return "warn", "未找到 SSH 主机私钥", ""
			}
			var bad []string
			for _, m := range matches {
				if perm, ok := filePerm(m); ok && perm&0o077 != 0 {
					bad = append(bad, fmt.Sprintf("%s(%o)", m, perm))
				}
			}
			if len(bad) == 0 {
				return "pass", fmt.Sprintf("%d 个私钥权限正确", len(matches)), ""
			}
			return "fail", "私钥权限过宽：" + strings.Join(bad, ", "), "chmod 600 /etc/ssh/ssh_host_*_key"
		}},
		{key: "world_writable_etc", category: "perm", title: "/etc 下没有全局可写文件", weight: 2, run: func() (string, string, string) {
			var bad []string
			entries, err := os.ReadDir("/etc")
			if err != nil {
				return "warn", "无法读取 /etc", ""
			}
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				if info, err := e.Info(); err == nil && info.Mode().Perm()&0o002 != 0 {
					bad = append(bad, e.Name())
					if len(bad) > 5 {
						break
					}
				}
			}
			if len(bad) == 0 {
				return "pass", "未发现全局可写配置文件", ""
			}
			return "fail", "全局可写：" + strings.Join(bad, ", "), "chmod o-w 去掉全局写权限"
		}},

		// --- 内核参数 -----------------------------------------------------
		{key: "kern_ip_forward", category: "kernel", title: "未开启 IP 转发（非路由主机）", weight: 1, run: sysctlIsZero("net.ipv4.ip_forward")},
		{key: "kern_accept_redirects", category: "kernel", title: "忽略 ICMP 重定向", weight: 2, run: sysctlIsZero("net.ipv4.conf.all.accept_redirects")},
		{key: "kern_accept_source_route", category: "kernel", title: "忽略源路由包", weight: 2, run: sysctlIsZero("net.ipv4.conf.all.accept_source_route")},
		{key: "kern_rp_filter", category: "kernel", title: "开启反向路径过滤", weight: 1, run: sysctlIsOne("net.ipv4.conf.all.rp_filter")},
		{key: "kern_syncookies", category: "kernel", title: "开启 SYN Cookie 防护", weight: 2, run: sysctlIsOne("net.ipv4.tcp_syncookies")},
		{key: "kern_aslr", category: "kernel", title: "地址随机化已开启", weight: 2, run: func() (string, string, string) {
			v := readTrim("/proc/sys/kernel/randomize_va_space")
			if v == "2" {
				return "pass", "randomize_va_space = 2", ""
			}
			return "fail", "randomize_va_space = " + v, "sysctl -w kernel.randomize_va_space=2"
		}},
		{key: "kern_core_dump", category: "kernel", title: "限制核心转储", weight: 1, run: func() (string, string, string) {
			v := readTrim("/proc/sys/fs/suid_dumpable")
			if v == "0" {
				return "pass", "suid_dumpable = 0", ""
			}
			return "warn", "suid_dumpable = " + v, "设为 0，避免 SUID 程序转储泄露内存"
		}},

		// --- 审计与日志 ---------------------------------------------------
		{key: "auditd_active", category: "audit", title: "auditd 正在运行", weight: 2, run: serviceRunning("auditd", "审计缺失时无法追溯越权操作")},
		{key: "rsyslog_active", category: "audit", title: "系统日志服务正在运行", weight: 2, run: serviceRunning("rsyslog", "日志服务停掉后事故现场会丢失")},
		{key: "log_dir_perm", category: "audit", title: "/var/log 权限 ≤ 755", weight: 1, run: permAtMost("/var/log", 0o755)},
		{key: "logrotate_present", category: "audit", title: "配置了日志轮转", weight: 1, run: func() (string, string, string) {
			if _, err := os.Stat("/etc/logrotate.conf"); err == nil {
				return "pass", "存在 /etc/logrotate.conf", ""
			}
			return "warn", "未找到 logrotate 配置", "日志不轮转会写满磁盘，进而影响服务"
		}},

		// --- 服务与暴露面 -------------------------------------------------
		{key: "dangerous_services", category: "service", title: "未运行 telnet/ftp 等明文服务", weight: 3, run: func() (string, string, string) {
			var bad []string
			for _, svc := range []string{"telnet", "telnet.socket", "vsftpd", "proftpd", "tftp"} {
				if isServiceActive(svc) {
					bad = append(bad, svc)
				}
			}
			if len(bad) == 0 {
				return "pass", "未发现明文传输服务", ""
			}
			return "fail", "运行中的明文服务：" + strings.Join(bad, ", "), "停用并改用 SSH/SFTP"
		}},
		{key: "db_exposed", category: "service", title: "数据库/缓存端口未对 0.0.0.0 暴露", weight: 3, run: func() (string, string, string) {
			var exposed []string
			for port, name := range map[int]string{3306: "mysql", 6379: "redis", 27017: "mongodb", 9200: "elasticsearch", 5432: "postgresql"} {
				if listeningOnAll(port) {
					exposed = append(exposed, fmt.Sprintf("%s:%d", name, port))
				}
			}
			if len(exposed) == 0 {
				return "pass", "未发现数据库端口对外监听", ""
			}
			return "fail", "对外监听：" + strings.Join(exposed, ", "), "改为监听 127.0.0.1 或加防火墙白名单"
		}},
		{key: "docker_socket_perm", category: "service", title: "Docker socket 权限受控", weight: 2, run: func() (string, string, string) {
			perm, ok := filePerm("/var/run/docker.sock")
			if !ok {
				return "pass", "未安装 Docker", ""
			}
			if perm&0o006 != 0 {
				return "fail", fmt.Sprintf("/var/run/docker.sock 权限 %o", perm), "docker 组成员等同于 root，去掉 others 写权限"
			}
			return "pass", fmt.Sprintf("/var/run/docker.sock 权限 %o", perm), ""
		}},
	}
}

// --- 共用辅助 -------------------------------------------------------------

func passwdLines() []string {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil
	}
	return strings.Split(strings.TrimSpace(string(data)), "\n")
}

func readTrim(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func atoiOrZero(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

func filepathGlob(pattern string) ([]string, error) {
	// filepath.Glob is only used with fixed patterns here; keeping the wrapper
	// makes the call sites read better.
	return filepathGlobImpl(pattern)
}

// sshdNumeric builds a "value ≤ max" check for an sshd directive.
func sshdNumeric(key string, max int, atMost bool) func() (string, string, string) {
	return func() (string, string, string) {
		v := sshdValue(key)
		if v == "" {
			return "warn", key + " 未显式配置", fmt.Sprintf("建议在 sshd_config 设置 %s %d", key, max)
		}
		// LoginGraceTime may carry a unit suffix (60s, 1m).
		n := atoiOrZero(strings.TrimRight(v, "smh"))
		if n == 0 {
			return "warn", key + " = " + v, ""
		}
		if n <= max {
			return "pass", fmt.Sprintf("%s = %s", key, v), ""
		}
		return "warn", fmt.Sprintf("%s = %s（建议 ≤ %d）", key, v, max), fmt.Sprintf("设置 %s %d", key, max)
	}
}

// permAtMost checks that a path's permission bits are no wider than want.
func permAtMost(path string, want uint32) func() (string, string, string) {
	return func() (string, string, string) {
		perm, ok := filePerm(path)
		if !ok {
			return "warn", "无法读取 " + path + " 权限", ""
		}
		if perm&^want == 0 {
			return "pass", fmt.Sprintf("%s 权限 %o", path, perm), ""
		}
		return "fail", fmt.Sprintf("%s 权限 %o（应 ≤ %o）", path, perm, want), fmt.Sprintf("chmod %o %s", want, path)
	}
}

func sysctlIsZero(key string) func() (string, string, string) {
	return func() (string, string, string) {
		v := readTrim("/proc/sys/" + strings.ReplaceAll(key, ".", "/"))
		if v == "" {
			return "warn", key + " 不可读", ""
		}
		if v == "0" {
			return "pass", key + " = 0", ""
		}
		return "warn", key + " = " + v, fmt.Sprintf("sysctl -w %s=0", key)
	}
}

func sysctlIsOne(key string) func() (string, string, string) {
	return func() (string, string, string) {
		v := readTrim("/proc/sys/" + strings.ReplaceAll(key, ".", "/"))
		if v == "" {
			return "warn", key + " 不可读", ""
		}
		if v == "1" {
			return "pass", key + " = 1", ""
		}
		return "warn", key + " = " + v, fmt.Sprintf("sysctl -w %s=1", key)
	}
}

func serviceRunning(name, why string) func() (string, string, string) {
	return func() (string, string, string) {
		if isServiceActive(name) {
			return "pass", name + " 运行中", ""
		}
		return "warn", name + " 未运行", why
	}
}

func isServiceActive(name string) bool {
	out, err := exec.Command("systemctl", "is-active", name).Output()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "active"
}

// listeningOnAll reports whether a port is bound to 0.0.0.0 (or ::), which is
// what turns a local database into an internet-facing one.
func listeningOnAll(port int) bool {
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n")[1:] {
			fields := strings.Fields(line)
			if len(fields) < 4 {
				continue
			}
			// local_address is HEXIP:HEXPORT; state 0A = LISTEN
			parts := strings.Split(fields[1], ":")
			if len(parts) != 2 {
				continue
			}
			p, err := strconv.ParseInt(parts[1], 16, 32)
			if err != nil || int(p) != port {
				continue
			}
			if fields[3] != "0A" {
				continue
			}
			if strings.HasSuffix(parts[0], "00000000") || parts[0] == "00000000000000000000000000000000" {
				return true
			}
		}
	}
	return false
}

// filepathGlobImpl isolates the filepath import so the helpers above stay short.
func filepathGlobImpl(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}
