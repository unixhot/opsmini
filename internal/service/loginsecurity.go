package service

import "strings"

// Login security analysis.
//
// 两个平台的数据源完全不同：
//   - Unix：/var/log/auth.log 或 /var/log/secure 的文本，靠正则抓 sshd 行。
//   - Windows：安全事件日志（4624 登录成功 / 4625 登录失败），要通过事件
//     API 读取；文本日志在这里根本不存在，所以旧实现永远返回 0。
//
// 公共部分（类型、SSH 加固建议）留在本文件，各自的数据源实现放在
// loginsecurity_unix.go / loginsecurity_windows.go。

// LoginSecurityService analyzes login security.
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
	// Message explains an empty report: no log source on this platform, or the
	// Windows security log needs administrator rights. Without it a page of zeros
	// looks like "nothing happened" instead of "cannot read".
	Message string `json:"message,omitempty"`
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

// Analyze produces the login security report from the platform's log source.
func (s *LoginSecurityService) Analyze() (*LoginReport, error) {
	report, err := s.analyze()
	if report == nil {
		report = &LoginReport{}
	}
	if report.BruteForce == nil {
		report.BruteForce = []BruteForce{}
	}
	if report.TopFailedIPs == nil {
		report.TopFailedIPs = []IPCount{}
	}
	if report.Anomalies == nil {
		report.Anomalies = []LoginAnomaly{}
	}
	return report, err
}

// SSHHardening dynamically generates SSH hardening suggestions based on the current sshd_config.
// Windows 上没有 sshd，返回空列表（前端会隐藏该区块）。
func (s *LoginSecurityService) SSHHardening() []SSHSuggestion {
	if !hasSSHD() {
		return []SSHSuggestion{}
	}
	return []SSHSuggestion{
		{Key: "root_login", Text: "禁止 root 直接登录（PermitRootLogin no）", Done: isYes(sshdValue("PermitRootLogin"))},
		{Key: "password_auth", Text: "关闭密码登录，改用密钥（PasswordAuthentication no）", Done: isNo(sshdValue("PasswordAuthentication"))},
		{Key: "change_port", Text: "修改默认端口 22（Port 2222）", Done: sshdValue("Port") != "" && sshdValue("Port") != "22"},
		{Key: "protocol2", Text: "仅使用 SSH 协议 v2", Done: sshdValue("Protocol") == "" || sshdValue("Protocol") == "2"},
	}
}

// isYes reports whether an sshd value means root login is disabled ("no" or "prohibit-password").
func isYes(v string) bool {
	return strings.EqualFold(v, "no") || strings.EqualFold(v, "prohibit-password")
}

// isNo reports whether an sshd value is exactly "no".
func isNo(v string) bool { return strings.EqualFold(v, "no") }
