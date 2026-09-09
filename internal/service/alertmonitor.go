package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"regexp"
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
	sys       *SystemService
	retention func() int            // returns alert event retention days (read from settings)
	settings  func() map[string]string // returns all settings (for SMTP config)

	stopCh chan struct{}
}

// NewAlertMonitor creates an AlertMonitor.
func NewAlertMonitor(ruleRepo *repository.AlertRuleRepo, eventRepo *repository.AlertEventRepo, notifRepo *repository.NotificationRepo, sys *SystemService, retention func() int, settings func() map[string]string) *AlertMonitor {
	return &AlertMonitor{
		ruleRepo:  ruleRepo,
		eventRepo: eventRepo,
		notifRepo: notifRepo,
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
		op, threshold := parseCondition(rule.Condition)
		if threshold <= 0 {
			continue
		}
		val, matched := metricValue(rule.Metric, sum, m.sys)
		if !matched {
			continue
		}
		if compareValue(val, op, threshold) {
			m.fire(rule, val, threshold, op)
		} else {
			m.resolve(rule)
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
	notify := rule.Notify
	has := func(kw string) bool { return strings.Contains(notify, kw) }
	// in-panel notification (bell)
	if has("面板") || has("站内") {
		if m.notifRepo != nil {
			notifLevel := model.NotifLevelWarn
			if level == "critical" {
				notifLevel = model.NotifLevelError
			}
			_ = m.notifRepo.Create(&model.Notification{
				Level:   notifLevel,
				Title:   "告警触发：" + rule.Name,
				Content: msg,
			})
		}
	}
	// email
	if has("邮件") || strings.Contains(strings.ToLower(notify), "email") {
		m.sendEmailAlert(rule.Name, level, msg)
	}
	// group-robot webhooks (WeCom / DingTalk / Feishu), per channel.
	m.sendWebhooks(notify, rule.Name, level, msg)
	log.Printf("ALERT fired: %s (rule=%s, id=%d)", msg, rule.Name, e.ID)
}

// resolve recovers an alert: marks the rule's active event as resolved and creates a
// linked recovery event so the history shows both when it fired and when it recovered.
func (m *AlertMonitor) resolve(rule model.AlertRule) {
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
		RuleID:     rule.ID,
		RuleName:   rule.Name,
		Metric:     rule.Metric,
		Level:      ev.Level,
		Message:    "已恢复：" + ev.Message,
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
func parseCondition(cond string) (string, float64) {
	m := conditionRe.FindStringSubmatch(strings.TrimSpace(cond))
	if len(m) < 3 {
		return ">=", 0
	}
	op := strings.TrimSpace(m[1])
	if op == "" {
		op = ">="
	}
	v, err := strconv.ParseFloat(m[2], 64)
	if err != nil {
		return op, 0
	}
	return op, v
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
func metricValue(metric string, sum *MonitorSummary, sys *SystemService) (float64, bool) {
	switch {
	case strings.Contains(metric, "CPU"):
		return sum.CPUPercent, true
	case strings.Contains(metric, "内存") || strings.Contains(metric, "Mem") || strings.Contains(metric, "Memory"):
		return sum.MemPercent, true
	case strings.Contains(metric, "磁盘") || strings.Contains(metric, "Disk"):
		return sum.DiskPercent, true
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
	switch {
	case strings.Contains(metric, "进程") || strings.Contains(metric, "Process"):
		return strconv.FormatFloat(v, 'f', 0, 64)
	case strings.Contains(metric, "下载") || strings.Contains(metric, "上传") || strings.Contains(metric, "下行") || strings.Contains(metric, "上行"):
		return strconv.FormatFloat(v, 'f', 1, 64) + " MB/s"
	default:
		return strconv.FormatFloat(v, 'f', 1, 64) + "%"
	}
}

// sendWebhooks sends the alert to the group-robot webhooks selected in the rule's notify field.
func (m *AlertMonitor) sendWebhooks(notify, ruleName, level, msg string) {
	if m.settings == nil {
		return
	}
	kv := m.settings()
	content := ruleName + ": " + msg
	targets := []struct{ key, kw, name string }{
		{"wecom_webhook", "企业微信", "WeCom"},
		{"dingtalk_webhook", "钉钉", "DingTalk"},
		{"feishu_webhook", "飞书", "Feishu"},
	}
	for _, t := range targets {
		if !strings.Contains(notify, t.kw) {
			continue
		}
		url := kv[t.key]
		if url == "" {
			continue
		}
		if err := sendWebhook(url, t.name, content); err != nil {
			log.Printf("%s webhook send failed: %v", t.name, err)
		}
	}
}

// sendWebhook posts a text message to a group-robot webhook, adapting the payload per platform.
func sendWebhook(url, platform, content string) error {
	var body map[string]interface{}
	if platform == "Feishu" {
		body = map[string]interface{}{"msg_type": "text", "content": map[string]string{"text": content}}
	} else {
		// WeCom and DingTalk share the same text format.
		body = map[string]interface{}{"msgtype": "text", "text": map[string]string{"content": content}}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}

// sendEmailAlert sends an email notification via SMTP if configured.
func (m *AlertMonitor) sendEmailAlert(ruleName, level, msg string) {
	if m.settings == nil {
		return
	}
	kv := m.settings()
	host := kv["smtp_host"]
	port := kv["smtp_port"]
	to := kv["smtp_to"]
	if host == "" || port == "" || to == "" {
		return
	}
	user := kv["smtp_user"]
	pass := kv["smtp_pass"]
	from := kv["smtp_from"]
	if from == "" {
		from = user
	}
	subject := "[OpsMini] Alert: " + ruleName + " (" + level + ")"
	body := "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		msg + "\r\n"
	addr := net.JoinHostPort(host, port)
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	if err := smtp.SendMail(addr, auth, from, []string{to}, []byte(body)); err != nil {
		log.Printf("alert email send failed: %v", err)
	}
}
