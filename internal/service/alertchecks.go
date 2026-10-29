package service

import (
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Event-type alert checks.
//
// Everything in alertmonitor.go watches a sampled value ("CPU 92% > 90%"). A
// whole class of real incidents has no such value: OOM killer fired, a container
// exited with code 1, last night's backup failed, a filesystem went read-only.
// Those used to be impossible to alert on, so the shipped rule set silently
// covered only half of what operators actually page for.
//
// The model used here is "how many such events happened inside the window". That
// single number plugs into the existing threshold engine unchanged, and it also
// gives free recovery: once the window slides past the last occurrence the value
// drops back to 0 and the alert resolves by itself.
//
// A check returns ok=false when the host simply cannot answer (no dmesg, no
// docker, no chrony). Callers must treat that as "unknown" and not fire — a rule
// that fires because a file is missing is worse than no rule.

// AlertSources carries the data sources that live outside the service package.
// They are function values so the alert monitor does not depend on the docker,
// backup or logpipe packages; the router wires them once.
type AlertSources struct {
	// DockerDaemon reports whether the docker daemon answers.
	DockerDaemon func() (up bool, detail string, ok bool)
	// CrashedContainers counts containers that exited non-zero inside window.
	CrashedContainers func(window time.Duration) (int, string, bool)
	// FailedCronJobs counts calendar/agent jobs whose last run failed.
	FailedCronJobs func() (int, string, bool)
	// FailedBackups counts failed backup runs inside window.
	FailedBackups func(window time.Duration) (int, string, bool)
	// LogPipeline reports backlog and dropped event counts.
	LogPipeline func() (pending, dropped int, ok bool)
}

// eventCheck is one event-type metric.
type eventCheck struct {
	metric string
	window time.Duration
	fn     func(window time.Duration) (int, string, bool)
}

// checkValue resolves a rule metric to a number, trying event checks first.
func (m *AlertMonitor) checkValue(metric string) (float64, bool) {
	for _, c := range m.eventChecks() {
		if c.metric != metric {
			continue
		}
		n, _, ok := c.fn(c.window)
		if !ok {
			return 0, false
		}
		return float64(n), true
	}
	return 0, false
}

// eventChecks builds the table bound to the current sources.
func (m *AlertMonitor) eventChecks() []eventCheck {
	src := m.sources
	hour := time.Hour
	day := 24 * time.Hour
	checks := []eventCheck{
		{metric: "NTP 时间偏移", window: 0, fn: func(time.Duration) (int, string, bool) { return ntpOffsetCheck() }},
		{metric: "只读文件系统数", window: 0, fn: func(time.Duration) (int, string, bool) { return readOnlyFS() }},
		{metric: "文件系统错误数", window: hour, fn: dmesgCount(`(?i)EXT4-fs error|XFS .*error|Remounting filesystem read-only|metadata I/O error`)},
		{metric: "OOM 事件数", window: hour, fn: dmesgCount(`(?i)Out of memory: Killed process|oom-kill:|invoked oom-killer`)},
		{metric: "硬件错误数", window: day, fn: dmesgCount(`(?i)Machine check|Hardware Error|EDAC.*error|PCIe Bus Error`)},
	}
	checks = append(checks, eventCheck{metric: "Docker 服务不可用", fn: func(time.Duration) (int, string, bool) {
		if src.DockerDaemon == nil {
			return 0, "", false
		}
		up, detail, ok := src.DockerDaemon()
		if !ok {
			return 0, "", false
		}
		if up {
			return 0, "", true
		}
		return 1, detail, true
	}})
	checks = append(checks, eventCheck{metric: "容器异常退出数", window: hour, fn: func(w time.Duration) (int, string, bool) {
		if src.CrashedContainers == nil {
			return 0, "", false
		}
		return src.CrashedContainers(w)
	}})
	checks = append(checks, eventCheck{metric: "计划任务失败数", window: 0, fn: func(time.Duration) (int, string, bool) {
		if src.FailedCronJobs == nil {
			return 0, "", false
		}
		return src.FailedCronJobs()
	}})
	checks = append(checks, eventCheck{metric: "备份失败数", window: day, fn: func(w time.Duration) (int, string, bool) {
		if src.FailedBackups == nil {
			return 0, "", false
		}
		return src.FailedBackups(w)
	}})
	checks = append(checks, eventCheck{metric: "日志管道积压", window: 0, fn: func(time.Duration) (int, string, bool) {
		if src.LogPipeline == nil {
			return 0, "", false
		}
		pending, dropped, ok := src.LogPipeline()
		if !ok {
			return 0, "", false
		}
		if dropped > 0 {
			// Drops are permanent data loss, so they dominate the backlog number.
			return dropped, "已丢弃 " + strconv.Itoa(dropped) + " 条，积压 " + strconv.Itoa(pending) + " 批", true
		}
		return pending, "", true
	}})
	return checks
}

// --- /proc and command based checks ---------------------------------------

var dmesgTS = regexp.MustCompile(`^\[\s*(\d+\.\d+)\]`)

// dmesgCount counts kernel messages matching pattern inside the last window.
// Kernel timestamps are seconds since boot, so the age of a line is uptime minus
// its timestamp — no log rotation or timezone involved.
func dmesgCount(pattern string) func(time.Duration) (int, string, bool) {
	re := regexp.MustCompile(pattern)
	return func(window time.Duration) (int, string, bool) {
		out, err := exec.Command("dmesg").Output()
		if err != nil {
			return 0, "", false
		}
		up := uptimeSeconds()
		if up <= 0 {
			return 0, "", false
		}
		n := 0
		last := ""
		for _, line := range strings.Split(string(out), "\n") {
			m := dmesgTS.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			ts, err := strconv.ParseFloat(m[1], 64)
			if err != nil {
				continue
			}
			if window > 0 && up-ts > window.Seconds() {
				continue
			}
			if re.MatchString(line) {
				n++
				last = strings.TrimSpace(line[len(m[0]):])
			}
		}
		return n, last, true
	}
}

// realFilesystems are the types that actually hold user data, so "read-only"
// means a problem rather than a design decision.
//
// This started as a blacklist and promptly false-fired on the cloud host:
// systemd mounts /run/credentials/* as read-only ramfs, and snap packages are
// squashfs. Listing what matters is the safer direction — a filesystem nobody
// listed simply never alerts.
var realFilesystems = map[string]bool{
	"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "zfs": true,
	"f2fs": true, "jfs": true, "reiserfs": true,
	"vfat": true, "ntfs": true, "ntfs3": true, "exfat": true, "hfsplus": true,
	"cifs": true, "nfs": true, "nfs4": true, "overlay": true,
}

// readOnlyFS counts real filesystems mounted read-only.
func readOnlyFS() (int, string, bool) {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return 0, "", false
	}
	n := 0
	first := ""
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		dev, mp, fstype, opts := f[0], f[1], f[2], f[3]
		if !realFilesystems[fstype] {
			continue
		}
		if !strings.Contains(","+opts+",", ",ro,") {
			continue
		}
		// A loop-mounted image or a snap revision is legitimately read-only.
		if strings.HasPrefix(dev, "/dev/loop") || strings.HasPrefix(mp, "/snap") ||
			strings.HasPrefix(mp, "/run/credentials") {
			continue
		}
		n++
		if first == "" {
			first = mp
		}
	}
	detail := ""
	if first != "" {
		detail = "挂载点 " + first
	}
	return n, detail, true
}

var offsetUnits = map[string]float64{
	"s": 1, "ms": 1e-3, "us": 1e-6, "µs": 1e-6, "ns": 1e-9,
}

// ntpOffsetCheck returns the absolute clock offset in seconds.
//
// Two sources, because a host has one or the other: systemd-timesyncd exposes it
// via timedatectl, chrony only through its CLI. If neither is present the check is
// unknown rather than "0 seconds" — otherwise the rule would look healthy on a
// host whose clock is not synchronised at all.
func ntpOffsetCheck() (int, string, bool) {
	if out, err := exec.Command("timedatectl", "timesync-status", "--property=Offset", "--value").Output(); err == nil {
		raw := strings.TrimSpace(string(out))
		if sec, ok := parseOffset(raw); ok {
			return int(sec), raw, true
		}
	}
	if out, err := exec.Command("chronyc", "tracking").Output(); err == nil {
		for _, line := range strings.Split(string(out), "\n") {
			if !strings.HasPrefix(strings.TrimSpace(line), "System time") {
				continue
			}
			// "System time     : 0.000012345 seconds slow of NTP time"
			f := strings.Fields(line)
			if len(f) < 4 {
				continue
			}
			if v, err := strconv.ParseFloat(f[3], 64); err == nil {
				sec := int(absFloat(v))
				if sec == 0 && absFloat(v) > 0.5 {
					sec = 1
				}
				return sec, strings.Join(f[3:], " "), true
			}
		}
	}
	return 0, "", false
}

// parseOffset turns "1.234ms" / "-3.2s" / "123us" into seconds.
func parseOffset(raw string) (float64, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, false
	}
	i := 0
	for i < len(raw) && (raw[i] == '-' || raw[i] == '+' || raw[i] == '.' || (raw[i] >= '0' && raw[i] <= '9')) {
		i++
	}
	num, err := strconv.ParseFloat(raw[:i], 64)
	if err != nil {
		return 0, false
	}
	unit := strings.TrimSpace(raw[i:])
	mult, ok := offsetUnits[unit]
	if !ok {
		// Bare number: timedatectl can print "0" when the offset is zero.
		return absFloat(num), true
	}
	return absFloat(num) * mult, true
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// eventMetricUnit reports the unit suffix for a counter-style metric.
//
// The suffix matters more than it looks: "上下文切换次数 95824551256.0%" reads as a
// percentage of something, and an operator cannot tell whether 95824551256 is
// alarming. Counters must print as integers with a countable unit.
func eventMetricUnit(metric string) (string, bool) {
	switch {
	case strings.Contains(metric, "上下文切换"), strings.Contains(metric, "错误"):
		return " 次", true
	case strings.Contains(metric, "连接数"), strings.Contains(metric, "进程数"):
		return " 个", true
	case strings.HasSuffix(metric, "数"):
		// Event metrics: OOM 事件数 / 备份失败数 / 只读文件系统数 …
		return " 次", true
	case metric == "NTP 时间偏移":
		return " 秒", true
	case metric == "日志管道积压":
		return " 批", true
	case metric == "Docker 服务不可用":
		return "", true
	}
	return "", false
}
