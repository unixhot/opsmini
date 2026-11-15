//go:build windows

package service

import "strings"

// Windows 的数据源是安全事件日志：
//   4624 = 登录成功，4625 = 登录失败。
// 文本形式的事件条目会被本地化，所以通过 ToXml() 拿结构化数据，再按 Data 的
// Name 属性取值 —— 属性名在事件清单里是英文，不随系统语言变化。

// hasSSHD reports whether SSH hardening advice applies on this platform.
func hasSSHD() bool { return false }

type winLogonEvent struct {
	Time string `json:"Time"`
	ID   int    `json:"Id"`
	User string `json:"User"`
	IP   string `json:"Ip"`
}

// analyze reads the last 24 hours of logon events.
//
// Security 日志默认只有管理员可读。读不到时返回一条带原因的 Message，而不是
// 一屏 0 —— 否则"没有异常"和"没有权限"在页面上长得一模一样。
func (s *LoginSecurityService) analyze() (*LoginReport, error) {
	report := &LoginReport{LogFile: "Security 事件日志（4624 成功 / 4625 失败）"}

	const q = `Get-WinEvent -FilterHashtable @{LogName='Security'; Id=4624,4625; StartTime=(Get-Date).AddHours(-24)} -MaxEvents 2000 -ErrorAction Stop | ForEach-Object { ` +
		`$x = [xml]$_.ToXml(); $d = @{}; ` +
		`$x.Event.EventData.Data | ForEach-Object { if ($_.Name) { $d[$_.Name] = $_.'#text' } }; ` +
		`[pscustomobject]@{ Time = $_.TimeCreated.ToString('yyyy-MM-dd HH:mm:ss'); Id = $_.Id; User = [string]$d['TargetUserName']; Ip = [string]$d['IpAddress'] } }`

	var events []winLogonEvent
	if err := psJSON(q, &events); err != nil {
		report.Message = "无法读取安全事件日志（需要以管理员身份运行）：" + err.Error()
		return report, nil
	}
	if len(events) == 0 {
		report.Message = "过去 24 小时没有登录事件记录"
		return report, nil
	}

	failByIP := map[string]int{}
	failByUser := map[string]map[string]bool{}

	for _, e := range events {
		user := normalizeAccount(e.User)
		ip := normalizeIP(e.IP)
		switch e.ID {
		case 4625:
			failByIP[ip]++
			if failByUser[user] == nil {
				failByUser[user] = map[string]bool{}
			}
			failByUser[user][ip] = true
			report.FailedCount24h++
		case 4624:
			report.SuccessCount24h++
			// 与 Linux 上"root 直接 SSH 登录"对应：管理员账户从远程登录。
			if strings.EqualFold(user, "Administrator") && isRemoteIP(ip) {
				report.Anomalies = append(report.Anomalies, LoginAnomaly{
					Time: e.Time, User: user, IP: ip, Reason: "管理员账户从远程登录",
				})
			}
		}
	}

	report.BruteForce = bruteForceFrom(failByUser, failByIP)
	report.TopFailedIPs = topIPs(failByIP, 10)
	if report.Message == "" && report.FailedCount24h == 0 && report.SuccessCount24h == 0 {
		report.Message = "过去 24 小时没有登录事件记录"
	}
	return report, nil
}

// normalizeAccount strips the machine/domain prefix ("HOST\user" -> "user") and
// drops the placeholder names Windows writes for system logons.
func normalizeAccount(u string) string {
	u = strings.TrimSpace(u)
	if u == "" || u == "-" {
		return "(未知)"
	}
	if i := strings.LastIndex(u, `\`); i >= 0 {
		u = u[i+1:]
	}
	switch strings.ToLower(u) {
	case "system", "local service", "network service", "dwm-1", "umfd-0":
		return "(系统账户)"
	}
	if strings.HasSuffix(u, "$") { // 计算机账户
		return "(计算机账户)"
	}
	return u
}

// normalizeIP maps the "-" placeholder to a readable label.
func normalizeIP(ip string) string {
	ip = strings.TrimSpace(ip)
	if ip == "" || ip == "-" || ip == "::1" || ip == "127.0.0.1" {
		return "本机"
	}
	return ip
}

// isRemoteIP reports whether the address is a real remote source.
func isRemoteIP(ip string) bool {
	if ip == "" || ip == "本机" || ip == "-" {
		return false
	}
	return true
}
