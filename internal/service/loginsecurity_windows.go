//go:build windows

package service

import (
	"strings"
	"time"
)

// loginQueryTimeout 比常规的 10 秒宽得多：2000 条安全事件逐条 ToXml() 再解析
// XML，慢机器上单这一项就要十几秒，用默认超时会把「慢」变成「拿不到数据」。
const loginQueryTimeout = 60 * time.Second

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
	// LogonType 是 4624 的登录类型：2 交互、3 网络、4 批处理、5 服务、7 解锁、
	// 8 明文网络、9 新凭据、10 远程交互、11 缓存交互。
	LogonType int `json:"LogonType"`
}

// analyze reads the last 24 hours of logon events.
//
// Security 日志默认只有管理员可读。读不到时返回一条带原因的 Message，而不是
// 一屏 0 —— 否则"没有异常"和"没有权限"在页面上长得一模一样。
func (s *LoginSecurityService) analyze() (*LoginReport, error) {
	report := &LoginReport{LogFile: "Security 事件日志（4624 成功 / 4625 失败）"}

	// 「没有匹配的事件」在 Get-WinEvent 里是一个终止错误，且提示文案会随系统语言
	// 变化（英文 "No events were found…"、中文「找不到任何…」）。按文案匹配只能
	// 覆盖有限几种语言，所以改成按 FullyQualifiedErrorId 判断 —— 它是 PowerShell
	// 的稳定标识（NoMatchingEventsFound），不随本地化改变。
	//
	// 其它错误照旧抛出，让调用方拿到 stderr（例如真正没有读日志权限时）。
	const q = `$e = try { Get-WinEvent -FilterHashtable @{LogName='Security'; Id=4624,4625; StartTime=(Get-Date).AddHours(-24)} -MaxEvents 2000 -ErrorAction Stop } ` +
		`catch { if ($_.FullyQualifiedErrorId -like 'NoMatchingEventsFound*') { @() } else { throw } }; ` +
		`$e | ForEach-Object { ` +
		`$x = [xml]$_.ToXml(); $d = @{}; ` +
		`$x.Event.EventData.Data | ForEach-Object { if ($_.Name) { $d[$_.Name] = $_.'#text' } }; ` +
		`[pscustomobject]@{ Time = $_.TimeCreated.ToString('yyyy-MM-dd HH:mm:ss'); Id = $_.Id; User = [string]$d['TargetUserName']; Ip = [string]$d['IpAddress']; LogonType = [int]$d['LogonType'] } }`

	var events []winLogonEvent
	// 2000 条事件逐条 ToXml() + XML 解析是这条链上最贵的一步，慢机器上远超 10 秒。
	if err := psJSONWithin(q, &events, loginQueryTimeout); err != nil {
		// 正常情况下「没有匹配事件」已经在脚本里被 catch 成空结果，走不到这里。
		// 再按文案兜一次底，是为了防某些 PowerShell 版本给出不同的错误标识。
		if isNoEventsError(err) {
			report.Message = "过去 24 小时没有登录事件记录"
			return report, nil
		}
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
			// 只统计「人真的登进来了」的类型：服务启动（5）、计划任务（4）、
			// 锁屏解锁（7）同样会写 4624，全算进来会让成功登录数虚高一个量级。
			if isHumanLogon(e.LogonType) {
				report.SuccessCount24h++
			}
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

// isHumanLogon reports whether a 4624 logon type means a person signed in.
//
// 2 interactive, 3 network, 8 network cleartext, 10 remote interactive,
// 11 cached interactive. Batches (4), services (5) and unlocks (7) are machine
// activity that would otherwise dominate the count on any long-running host.
// A missing field (0) is counted rather than dropped: under-reporting logins
// hides real activity, over-reporting only inflates a number that is visible.
func isHumanLogon(t int) bool {
	switch t {
	case 0, 2, 3, 8, 10, 11:
		return true
	default:
		return false
	}
}

// isNoEventsError 是「没有匹配事件」的兜底判断。
//
// 主路径不靠它：脚本里用 FullyQualifiedErrorId（NoMatchingEventsFound）在
// PowerShell 内部就把这种情况归成空结果，那个标识不随系统语言变化。这里按文案
// 匹配只覆盖中英文，其余语言（日/德/韩…）识别不到时会退回「无法读取」的提示 ——
// 属于已知局限，不是主判定路径。
func isNoEventsError(err error) bool {
	msg := strings.ToLower(err.Error())
	for _, s := range []string{
		"no events were found",
		"no events found",
		"未找到任何事件",
		"找不到任何", // 中文（简/繁）的实际措辞是「找不到任何与指定的选择条件匹配的事件」
	} {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
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
