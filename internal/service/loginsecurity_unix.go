//go:build !windows

package service

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Unix 的数据源是文本认证日志：/var/log/auth.log（Debian 系）或
// /var/log/secure（RHEL 系）。只读最后 5000 行，避免大日志把内存打满。

var (
	reFailedPass = regexp.MustCompile(`Failed password for (?:invalid user )?([^\s]+) from ([\d.]+)`)
	reInvalidUsr = regexp.MustCompile(`Invalid user ([^\s]+) from ([\d.]+)`)
	reAccepted   = regexp.MustCompile(`Accepted (?:password|publickey) for ([^\s]+) from ([\d.]+)`)
	reSshdLine   = regexp.MustCompile(`sshd`)
	reTimestamp  = regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d+\s+\d{2}:\d{2}:\d{2})`)
)

// hasSSHD reports whether SSH hardening advice applies on this platform.
func hasSSHD() bool { return true }

// logFile returns the path of the auth log file that actually exists.
func logFile() string {
	for _, f := range []string{"/var/log/auth.log", "/var/log/secure"} {
		if fi, err := os.Stat(f); err == nil && !fi.IsDir() {
			return f
		}
	}
	return ""
}

// analyze parses auth logs into brute-force / anomalous-login findings.
func (s *LoginSecurityService) analyze() (*LoginReport, error) {
	path := logFile()
	report := &LoginReport{LogFile: path}
	if path == "" {
		report.Message = "未找到认证日志（/var/log/auth.log 或 /var/log/secure）"
		return report, nil
	}
	out, err := exec.Command("tail", "-n", "5000", path).Output()
	if err != nil {
		return report, err
	}

	failByIP := map[string]int{}
	failByUser := map[string]map[string]bool{} // user -> set(ip)

	for _, line := range strings.Split(string(out), "\n") {
		if !reSshdLine.MatchString(line) {
			continue
		}
		var ts string
		if m := reTimestamp.FindStringSubmatch(line); m != nil {
			ts = m[1]
		}
		if m := reFailedPass.FindStringSubmatch(line); m != nil {
			recordFailure(report, failByIP, failByUser, m[1], m[2])
		} else if m := reInvalidUsr.FindStringSubmatch(line); m != nil {
			recordFailure(report, failByIP, failByUser, m[1], m[2])
		} else if m := reAccepted.FindStringSubmatch(line); m != nil {
			user, ip := m[1], m[2]
			report.SuccessCount24h++
			if user == "root" {
				report.Anomalies = append(report.Anomalies, LoginAnomaly{
					Time: ts, User: user, IP: ip, Reason: "root 账户直接 SSH 登录",
				})
			}
		}
	}

	report.BruteForce = bruteForceFrom(failByUser, failByIP)
	report.TopFailedIPs = topIPs(failByIP, 10)
	return report, nil
}

// recordFailure accumulates one failed login, shared by both failure patterns.
func recordFailure(report *LoginReport, failByIP map[string]int, failByUser map[string]map[string]bool, user, ip string) {
	failByIP[ip]++
	if failByUser[user] == nil {
		failByUser[user] = map[string]bool{}
	}
	failByUser[user][ip] = true
	report.FailedCount24h++
}
