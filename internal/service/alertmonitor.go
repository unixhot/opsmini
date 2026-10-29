package service

import (
	"log"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// AlertMonitor is the background alert detector: it periodically evaluates enabled
// alert rules, records alert events on trigger, marks them resolved on recovery,
// and cleans up expired events periodically.
type AlertMonitor struct {
	ruleRepo  *repository.AlertRuleRepo
	eventRepo *repository.AlertEventRepo
	notifRepo *repository.NotificationRepo
	notifier  *Notifier
	sys       *SystemService
	retention func() int               // returns alert event retention days (read from settings)
	settings  func() map[string]string // returns all settings (for SMTP config)

	stopCh chan struct{}

	// sources supplies the event-type data sources (docker / cron / backup /
	// log pipeline). Nil entries simply make those rules "unknown" instead of
	// guessing, which matters on hosts that do not run docker at all.
	sources AlertSources

	// onFire is called after an alert event is created and notifications sent.
	// The perception layer uses it to turn metric alerts into signals instead of
	// polling the same metrics twice.
	onFire func(rule model.AlertRule, val, threshold float64, op string)
}

// SetSources wires the external data sources used by event-type rules.
func (m *AlertMonitor) SetSources(src AlertSources) { m.sources = src }

// SetOnFire registers a hook invoked whenever an alert fires.
func (m *AlertMonitor) SetOnFire(fn func(rule model.AlertRule, val, threshold float64, op string)) {
	m.onFire = fn
}

// NewAlertMonitor creates an AlertMonitor.
func NewAlertMonitor(ruleRepo *repository.AlertRuleRepo, eventRepo *repository.AlertEventRepo, notifRepo *repository.NotificationRepo, sys *SystemService, retention func() int, settings func() map[string]string) *AlertMonitor {
	return &AlertMonitor{
		ruleRepo:  ruleRepo,
		eventRepo: eventRepo,
		notifRepo: notifRepo,
		notifier:  NewNotifier(notifRepo, settings),
		sys:       sys,
		retention: retention,
		settings:  settings,
		stopCh:    make(chan struct{}),
	}
}

// Start starts the background detection loop.
func (m *AlertMonitor) Start() {
	go m.loop()
}

// Stop stops detection.
func (m *AlertMonitor) Stop() { close(m.stopCh) }

// loop evaluates alert rules periodically (every 60s) and cleans up expired events hourly.
func (m *AlertMonitor) loop() {
	// evaluate once immediately, then every 60s; clean up expired events every hour.
	m.evaluate()
	m.cleanup()
	checkTicker := time.NewTicker(60 * time.Second)
	cleanTicker := time.NewTicker(time.Hour)
	defer checkTicker.Stop()
	defer cleanTicker.Stop()
	for {
		select {
		case <-m.stopCh:
			return
		case <-checkTicker.C:
			m.evaluate()
		case <-cleanTicker.C:
			m.cleanup()
		}
	}
}

// evaluate evaluates all enabled rules and triggers or resolves events.
func (m *AlertMonitor) evaluate() {
	rules, err := m.ruleRepo.List()
	if err != nil {
		return
	}
	sum, err := m.sys.MonitorSummary()
	if err != nil {
		return
	}
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		op, threshold, ok := parseCondition(rule.Condition)
		if !ok {
			// Relative thresholds ("系统负载 > CPU 核数") cannot be parsed from the
			// string; resolve them from the host. Without this the threshold came
			// out as 0 and the rule was skipped forever.
			if th, rok := relativeThreshold(rule.Metric); rok {
				threshold = th
			} else {
				continue
			}
		}
		// Event-type metrics (OOM killed, container crashed, backup failed) have no
		// sampled value; they report how many happened inside a window. Check
		// those first, then fall back to the sampled metrics.
		val, matched := m.checkValue(rule.Metric)
		if !matched {
			val, matched = metricValue(rule.Metric, sum, m.sys)
		}
		if !matched {
			continue
		}
		if compareValue(val, op, threshold) {
			m.fire(rule, val, threshold, op)
		} else {
			m.resolve(rule, val, threshold)
		}
	}
}

// fire triggers an alert: if the rule already has an active event, skip; otherwise create a new one.
func (m *AlertMonitor) fire(rule model.AlertRule, val, threshold float64, op string) {
	if _, err := m.eventRepo.FindActiveByRule(rule.ID); err == nil {
		return // already alerting
	}
	level := "warning"
	if threshold >= 90 {
		level = "critical"
	}
	msg := rule.Metric + " " + formatMetricValue(rule.Metric, val) + " " + op + " 阈值 " + formatMetricValue(rule.Metric, threshold)
	e := &model.AlertEvent{
		RuleID:   rule.ID,
		RuleName: rule.Name,
		Metric:   rule.Metric,
		Level:    level,
		Message:  msg,
		Kind:     model.AlertKindAlert,
		Status:   model.AlertStatusActive,
	}
	if err := m.eventRepo.Create(e); err != nil {
		log.Printf("alert event create: %v", err)
		return
	}
	// Route notifications by the rule's notify channels (comma-separated labels).
	// Delivery is delegated to the shared Notifier so alerts and AI tasks behave
	// identically on every channel.
	m.notifier.Send(NotifyMessage{
		Channels:    rule.Notify,
		Level:       level,
		PanelTitle:  "告警触发：" + rule.Name,
		PanelBody:   msg,
		MailSubject: "[OpsMini] Alert: " + rule.Name + " (" + level + ")",
		MailBody:    msg,
		WebhookText: rule.Name + ": " + msg,
	})
	log.Printf("ALERT fired: %s (rule=%s, id=%d)", msg, rule.Name, e.ID)
	if m.onFire != nil {
		m.onFire(rule, val, threshold, op)
	}
}

// resolve recovers an alert: marks the rule's active event as resolved and creates a
// linked recovery event so the history shows both when it fired and when it recovered.
func (m *AlertMonitor) resolve(rule model.AlertRule, val, threshold float64) {
	ev, err := m.eventRepo.FindActiveByRule(rule.ID)
	if err != nil {
		return
	}
	now := time.Now()
	ev.Status = model.AlertStatusResolved
	ev.ResolvedAt = &now
	if err := m.eventRepo.Update(ev); err != nil {
		log.Printf("alert event resolve: %v", err)
		return
	}
	rec := &model.AlertEvent{
		RuleID:   rule.ID,
		RuleName: rule.Name,
		Metric:   rule.Metric,
		Level:    ev.Level,
		// The recovery message must state the value that actually cleared the
		// threshold. Reusing ev.Message printed the firing value, so the history
		// read「已恢复：CPU 使用率 98.4% > 阈值 90.0%」— a recovered alert that
		// still claimed to be over the limit.
		Message:    "已恢复：" + rule.Metric + " " + formatMetricValue(rule.Metric, val) + "（阈值 " + formatMetricValue(rule.Metric, threshold) + "）",
		Kind:       model.AlertKindRecovery,
		RelatedID:  ev.ID,
		Status:     model.AlertStatusResolved,
		ResolvedAt: &now,
	}
	if err := m.eventRepo.Create(rec); err != nil {
		log.Printf("recovery event create: %v", err)
	}
	log.Printf("ALERT resolved: rule=%s event=%d", rule.Name, ev.ID)
}

// cleanup removes alert events older than the retention period.
func (m *AlertMonitor) cleanup() {
	days := 30
	if m.retention != nil {
		if d := m.retention(); d > 0 {
			days = d
		}
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	n, err := m.eventRepo.DeleteOlderThan(cutoff)
	if err != nil {
		log.Printf("alert cleanup: %v", err)
		return
	}
	if n > 0 {
		log.Printf("alert cleanup: removed %d events older than %d days", n, days)
	}
}

var conditionRe = regexp.MustCompile(`([><=]{1,2})?\s*(\d+(?:\.\d+)?)`)

// parseCondition parses an operator and threshold from a condition string (e.g. ">= 90" or "> 85%").
// The operator defaults to ">=" when omitted (backward compatible with old rules).
//
// The boolean reports whether a numeric threshold was actually found. Callers must
// distinguish "no number in the condition" from "threshold is zero": event rules
// legitimately use "> 0", and treating that as unparseable would disable them.
func parseCondition(cond string) (string, float64, bool) {
	m := conditionRe.FindStringSubmatch(strings.TrimSpace(cond))
	if len(m) < 3 {
		return ">=", 0, false
	}
	op := strings.TrimSpace(m[1])
	if op == "" {
		op = ">="
	}
	v, err := strconv.ParseFloat(m[2], 64)
	if err != nil {
		return op, 0, false
	}
	return op, v, true
}

// relativeThreshold resolves thresholds that are described in words rather than
// numbers, using the current host.
func relativeThreshold(metric string) (float64, bool) {
	if strings.Contains(metric, "负载") || strings.Contains(metric, "Load") {
		return float64(runtime.NumCPU()), true
	}
	return 0, false
}

// compareValue compares a metric value against the threshold using the operator.
func compareValue(val float64, op string, threshold float64) bool {
	switch op {
	case ">":
		return val > threshold
	case ">=":
		return val >= threshold
	case "<":
		return val < threshold
	case "<=":
		return val <= threshold
	case "=", "==":
		return val == threshold
	default:
		return val >= threshold
	}
}

// metricValue returns the current metric value and whether it matched.
//
// Order matters: several rule names contain another metric's name ("磁盘 inode"
// contains "磁盘", "CPU 温度" contains "CPU"), so the narrower match must come
// first. Getting this wrong makes a rule silently watch the wrong number — the
// CPU-temperature rule used to report CPU utilisation as degrees.
func metricValue(metric string, sum *MonitorSummary, sys *SystemService) (float64, bool) {
	switch {
	case strings.Contains(metric, "inode") || strings.Contains(metric, "Inode"):
		v, ok := procPercent("inode")
		if ok {
			return v, true
		}
		return sum.DiskPercent, true
	case strings.Contains(metric, "Swap") || strings.Contains(metric, "swap"):
		return procPercent("swap")
	case strings.Contains(metric, "负载") || strings.Contains(metric, "Load"):
		return sum.Load5, true
	case strings.Contains(metric, "TCP") || strings.Contains(metric, "连接数"):
		return procCount("tcp")
	case strings.Contains(metric, "上下文切换"):
		return procCount("ctxt")
	case strings.Contains(metric, "网卡接收错误"):
		return procCount("neterr")
	case strings.Contains(metric, "磁盘 IO") || strings.Contains(metric, "IO 使用率"):
		return procCount("ioutil")
	// Temperature must be matched before "CPU": every temperature rule name
	// contains "CPU" too, and matching it first made「CPU 温度」silently read the
	// CPU utilisation percentage (98.4% "hot" while the box was merely busy).
	case strings.Contains(metric, "温度") || strings.Contains(metric, "Temperature"):
		if sys == nil {
			return 0, false
		}
		return sys.CPUTemperature()
	case strings.Contains(metric, "CPU"):
		return sum.CPUPercent, true
	case strings.Contains(metric, "内存") || strings.Contains(metric, "Mem") || strings.Contains(metric, "Memory"):
		return sum.MemPercent, true
	case strings.Contains(metric, "磁盘") || strings.Contains(metric, "Disk"):
		return sum.DiskPercent, true
	case strings.Contains(metric, "僵尸"):
		return float64(sum.ZombieCount), true
	case strings.Contains(metric, "运行时长"):
		up := uptimeSeconds()
		if up <= 0 {
			return 0, false // 无 /proc/uptime：宁可不说，也不要误报
		}
		return up, true
	case strings.Contains(metric, "进程") || strings.Contains(metric, "Process"):
		return float64(sum.ProcessCount), true
	case strings.Contains(metric, "下载") || strings.Contains(metric, "下行"):
		r, _ := sys.NetRate()
		return float64(r) / 1024 / 1024, true // MB/s
	case strings.Contains(metric, "上传") || strings.Contains(metric, "上行"):
		_, s := sys.NetRate()
		return float64(s) / 1024 / 1024, true // MB/s
	}
	return 0, false
}

// formatMetricValue formats a metric value with the appropriate unit.
func formatMetricValue(metric string, v float64) string {
	// Counter and gauge event metrics: "3 次", "12 秒", "2 批", or no unit at all
	// for a 0/1 gauge like "Docker 服务不可用".
	if u, ok := eventMetricUnit(metric); ok {
		return strconv.FormatFloat(v, 'f', 0, 64) + u
	}
	switch {
	case strings.Contains(metric, "温度") || strings.Contains(metric, "Temperature"):
		return strconv.FormatFloat(v, 'f', 1, 64) + "°C"
	case strings.Contains(metric, "运行时长"):
		return strconv.FormatFloat(v, 'f', 0, 64) + " 秒"
	case strings.Contains(metric, "僵尸"):
		return strconv.FormatFloat(v, 'f', 0, 64)
	case strings.Contains(metric, "进程") || strings.Contains(metric, "Process"):
		return strconv.FormatFloat(v, 'f', 0, 64)
	case strings.Contains(metric, "下载") || strings.Contains(metric, "上传") || strings.Contains(metric, "下行") || strings.Contains(metric, "上行"):
		return strconv.FormatFloat(v, 'f', 1, 64) + " MB/s"
	default:
		return strconv.FormatFloat(v, 'f', 1, 64) + "%"
	}
}

// --- /proc based metric helpers -------------------------------------------
//
// The shipped rules cover more metrics than MonitorSummary carries, and several
// of them used to be silently dead: metricValue returned "no match", so the rule
// never fired. These helpers read the specific /proc file each metric needs.

// procPercent computes a percentage from /proc (inode usage or swap usage).
func procPercent(kind string) (float64, bool) {
	switch kind {
	case "swap":
		data, err := os.ReadFile("/proc/meminfo")
		if err != nil {
			return 0, false
		}
		var total, free float64
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) < 2 {
				continue
			}
			switch f[0] {
			case "SwapTotal:":
				total, _ = strconv.ParseFloat(f[1], 64)
			case "SwapFree:":
				free, _ = strconv.ParseFloat(f[1], 64)
			}
		}
		if total == 0 {
			return 0, true // no swap configured: 0% used, rule stays quiet
		}
		return (total - free) / total * 100, true
	case "inode":
		// Take the fullest filesystem: that is the one that will break.
		out, err := exec.Command("df", "-P", "-i").Output()
		if err != nil {
			return 0, false
		}
		worst := 0.0
		found := false
		for i, line := range strings.Split(string(out), "\n") {
			if i == 0 || strings.TrimSpace(line) == "" {
				continue
			}
			f := strings.Fields(line)
			if len(f) < 6 {
				continue
			}
			use := strings.TrimSuffix(f[4], "%")
			if v, err := strconv.ParseFloat(use, 64); err == nil {
				found = true
				if v > worst {
					worst = v
				}
			}
		}
		return worst, found
	}
	return 0, false
}

// procCount returns a raw count/rate from /proc.
func procCount(kind string) (float64, bool) {
	switch kind {
	case "tcp":
		data, err := os.ReadFile("/proc/net/tcp")
		if err != nil {
			return 0, false
		}
		n := 0
		for _, line := range strings.Split(string(data), "\n")[1:] {
			if strings.TrimSpace(line) != "" {
				n++
			}
		}
		return float64(n), true
	case "ctxt":
		// /proc/stat counts context switches since boot, so the raw number is
		// meaningless against a threshold (a two-week-old host reports ~1e11 and
		// alerts forever). The rule is written as "switches per second", which is
		// what actually indicates CPU thrashing, so convert with the uptime.
		data, err := os.ReadFile("/proc/stat")
		if err != nil {
			return 0, false
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "ctxt ") {
				v, err := strconv.ParseFloat(strings.TrimSpace(strings.TrimPrefix(line, "ctxt")), 64)
				if err != nil {
					return 0, false
				}
				up := uptimeSeconds()
				if up <= 0 {
					return 0, false
				}
				return v / up, true
			}
		}
		return 0, false
	case "neterr":
		data, err := os.ReadFile("/proc/net/dev")
		if err != nil {
			return 0, false
		}
		var total float64
		for _, line := range strings.Split(string(data), "\n")[2:] {
			f := strings.Fields(strings.ReplaceAll(line, ":", " "))
			if len(f) < 12 {
				continue
			}
			// fields: iface rx_bytes rx_packets rx_errs ... tx_bytes tx_packets tx_errs
			if rxErr, err := strconv.ParseFloat(f[3], 64); err == nil {
				total += rxErr
			}
			if txErr, err := strconv.ParseFloat(f[11], 64); err == nil {
				total += txErr
			}
		}
		return total, true
	case "ioutil":
		data, err := os.ReadFile("/proc/diskstats")
		if err != nil {
			return 0, false
		}
		// Field 10 (io_ticks, ms) over wall time gives busy percentage; without a
		// previous sample we can only report a coarse "busy ms in last interval",
		// so use the average since boot as a stable proxy.
		var ms float64
		for _, line := range strings.Split(string(data), "\n") {
			f := strings.Fields(line)
			if len(f) < 13 {
				continue
			}
			if v, err := strconv.ParseFloat(f[12], 64); err == nil {
				ms += v
			}
		}
		up := uptimeSeconds()
		if up <= 0 {
			return 0, false
		}
		return ms / (up * 1000) * 100, true
	}
	return 0, false
}

func uptimeSeconds() float64 {
	data, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(data))
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return v
}
