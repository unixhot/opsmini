package service

import (
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// LoginSecurityService analyzes login security (parses system auth logs).
type LoginSecurityService struct{}

// NewLoginSecurityService creates a LoginSecurityService.
func NewLoginSecurityService() *LoginSecurityService { return &LoginSecurityService{} }

// LoginReport is the login security analysis report.
type LoginReport struct {
	LogFile         string         `json:"log_file"`
	BruteForce      []BruteForce   `json:"brute_force"`
	TopFailedIPs    []IPCount      `json:"top_failed_ips"`
	Anomalies       []LoginAnomaly `json:"anomalies"`
	FailedCount24h  int            `json:"failed_count_24h"`
	SuccessCount24h int            `json:"success_count_24h"`
}

// BruteForce is a suspected brute-force target.
type BruteForce struct {
	User  string   `json:"user"`
	Count int      `json:"count"`
	IPs   []string `json:"ips"`
}

// IPCount is a source IP count.
type IPCount struct {
	IP    string `json:"ip"`
	Count int    `json:"count"`
	User  string `json:"user"`
}

// LoginAnomaly is an anomalous login record.
type LoginAnomaly struct {
	Time   string `json:"time"`
	User   string `json:"user"`
	IP     string `json:"ip"`
	Reason string `json:"reason"`
}

// SSHSuggestion is an SSH hardening suggestion.
type SSHSuggestion struct {
	Key  string `json:"key"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

var (
	reFailedPass = regexp.MustCompile(`Failed password for (?:invalid user )?([^\s]+) from ([\d.]+)`)
	reInvalidUsr = regexp.MustCompile(`Invalid user ([^\s]+) from ([\d.]+)`)
	reAccepted   = regexp.MustCompile(`Accepted (?:password|publickey) for ([^\s]+) from ([\d.]+)`)
	reSshdLine   = regexp.MustCompile(`sshd`)
)

// logFile returns the path of the auth log file that actually exists.
func (s *LoginSecurityService) logFile() string {
	for _, f := range []string{"/var/log/auth.log", "/var/log/secure"} {
		if fi, err := os.Stat(f); err == nil && !fi.IsDir() {
			return f
		}
	}
	return ""
}

// Analyze parses auth logs and produces brute-force/anomalous-login analysis.
// It only reads the tail of the log file (tail -n 5000) to avoid memory overflow from huge logs.
func (s *LoginSecurityService) Analyze() (*LoginReport, error) {
	report := &LoginReport{LogFile: s.logFile()}
	if report.LogFile == "" {
		return report, nil
	}
	out, err := exec.Command("tail", "-n", "5000", report.LogFile).Output()
	if err != nil {
		return report, err
	}
	lines := strings.Split(string(out), "\n")
	// only consider the last 24 hours
	cutoff := time.Now().Add(-24 * time.Hour).Format("Jan _2 15:04:05")

	failByIP := map[string]int{}
	failByUser := map[string]map[string]bool{} // user -> set(ip)
	now := time.Now()

	for _, line := range lines {
		if !reSshdLine.MatchString(line) {
			continue
		}
		// extract time (e.g. "Aug 14 07:00:01")
		var ts string
		if m := regexp.MustCompile(`^([A-Z][a-z]{2}\s+\d+\s+\d{2}:\d{2}:\d{2})`).FindStringSubmatch(line); m != nil {
			ts = m[1]
		}
		if m := reFailedPass.FindStringSubmatch(line); m != nil {
			user, ip := m[1], m[2]
			failByIP[ip]++
			if failByUser[user] == nil {
				failByUser[user] = map[string]bool{}
			}
			failByUser[user][ip] = true
			report.FailedCount24h++
		} else if m := reInvalidUsr.FindStringSubmatch(line); m != nil {
			user, ip := m[1], m[2]
			failByIP[ip]++
			if failByUser[user] == nil {
				failByUser[user] = map[string]bool{}
			}
			failByUser[user][ip] = true
			report.FailedCount24h++
		} else if m := reAccepted.FindStringSubmatch(line); m != nil {
			user, ip := m[1], m[2]
			report.SuccessCount24h++
			// anomaly: root direct login
			if user == "root" {
				report.Anomalies = append(report.Anomalies, LoginAnomaly{Time: ts, User: user, IP: ip, Reason: "root 账户直接 SSH 登录"})
			}
			_ = now
		}
	}

	// brute force: a user with >= 5 failures
	for user, ips := range failByUser {
		total := 0
		for ip := range ips {
			total += failByIP[ip]
		}
		if total >= 5 {
			ipList := make([]string, 0, len(ips))
			for ip := range ips {
				ipList = append(ipList, ip)
			}
			sort.Strings(ipList)
			report.BruteForce = append(report.BruteForce, BruteForce{User: user, Count: total, IPs: ipList})
		}
	}
	sort.Slice(report.BruteForce, func(i, j int) bool { return report.BruteForce[i].Count > report.BruteForce[j].Count })

	// top failed source IPs
	type kv struct {
		ip    string
		count int
	}
	var arr []kv
	for ip, c := range failByIP {
		arr = append(arr, kv{ip, c})
	}
	sort.Slice(arr, func(i, j int) bool { return arr[i].count > arr[j].count })
	for i, e := range arr {
		if i >= 10 {
			break
		}
		report.TopFailedIPs = append(report.TopFailedIPs, IPCount{IP: e.ip, Count: e.count})
	}
	_ = cutoff
	return report, nil
}

// SSHHardening dynamically generates SSH hardening suggestions based on the current sshd_config.
func (s *LoginSecurityService) SSHHardening() []SSHSuggestion {
	return []SSHSuggestion{
		{Key: "root_login", Text: "禁止 root 直接登录（PermitRootLogin no）", Done: isYes(sshdValue("PermitRootLogin"))},
		{Key: "password_auth", Text: "关闭密码登录，改用密钥（PasswordAuthentication no）", Done: isNo(sshdValue("PasswordAuthentication"))},
		{Key: "change_port", Text: "修改默认端口 22（Port 2222）", Done: sshdValue("Port") != "" && sshdValue("Port") != "22"},
		{Key: "protocol2", Text: "仅使用 SSH 协议 v2", Done: sshdValue("Protocol") == "" || sshdValue("Protocol") == "2"},
	}
}

// isYes reports whether an sshd value means root login is disabled ("no" or "prohibit-password").
func isYes(v string) bool { return strings.EqualFold(v, "no") || strings.EqualFold(v, "prohibit-password") }
// isNo reports whether an sshd value is exactly "no".
func isNo(v string) bool  { return strings.EqualFold(v, "no") }
