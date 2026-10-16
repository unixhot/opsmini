package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/smtp"
	"strings"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// Notification channel keys, matching the labels stored in the panel settings
// and in the per-rule / per-task `notify` field.
const (
	NotifyChannelPanel    = "面板"
	NotifyChannelEmail    = "邮件"
	NotifyChannelWeCom    = "企业微信"
	NotifyChannelDingTalk = "钉钉"
	NotifyChannelFeishu   = "飞书"
)

// NotifyChannels returns the selectable channels (used by the frontend via the
// permission-free settings payload and by tests).
func NotifyChannels() []string {
	return []string{NotifyChannelPanel, NotifyChannelEmail, NotifyChannelWeCom, NotifyChannelDingTalk, NotifyChannelFeishu}
}

// NotifyMessage is one message to deliver. Each channel gets its own rendering so
// a compact webhook text can coexist with a fuller in-panel / email body.
type NotifyMessage struct {
	// Channels is the raw comma-separated channel list (labels or English keys).
	Channels string
	// Level is info / warning / critical (drives the in-panel badge).
	Level string
	// PanelTitle / PanelBody feed the in-panel notification bell.
	PanelTitle string
	PanelBody  string
	// MailSubject / MailBody feed the SMTP email.
	MailSubject string
	MailBody    string
	// WebhookText feeds the WeCom / DingTalk / Feishu group robots.
	WebhookText string
}

// Notifier routes a message to the channels configured in the panel. It is shared
// by the alert monitor and by the AI agent's scheduled tasks so both use exactly
// the same delivery behaviour.
type Notifier struct {
	notifRepo *repository.NotificationRepo
	settings  func() map[string]string
	client    *http.Client
}

// NewNotifier creates a Notifier. settings may be nil (channels then degrade to
// the in-panel bell only).
func NewNotifier(notifRepo *repository.NotificationRepo, settings func() map[string]string) *Notifier {
	return &Notifier{notifRepo: notifRepo, settings: settings, client: &http.Client{}}
}

// Has reports whether the channel list enables the given channel.
func hasChannel(channels, label string) bool {
	if strings.Contains(channels, label) {
		return true
	}
	switch label {
	case NotifyChannelPanel:
		return strings.Contains(channels, "站内") || strings.Contains(strings.ToLower(channels), "panel")
	case NotifyChannelEmail:
		return strings.Contains(strings.ToLower(channels), "email") || strings.Contains(strings.ToLower(channels), "mail")
	case NotifyChannelWeCom:
		return strings.Contains(strings.ToLower(channels), "wecom")
	case NotifyChannelDingTalk:
		return strings.Contains(strings.ToLower(channels), "dingtalk")
	case NotifyChannelFeishu:
		return strings.Contains(strings.ToLower(channels), "feishu") || strings.Contains(strings.ToLower(channels), "lark")
	}
	return false
}

// Send delivers the message to every enabled channel and returns the channels
// that were actually attempted.
func (n *Notifier) Send(m NotifyMessage) []string {
	if n == nil {
		return nil
	}
	channels := strings.TrimSpace(m.Channels)
	if channels == "" {
		// No channel configured: deliver nothing. Callers that want a default
		// (the AI task runner defaults to the in-panel bell) pass it explicitly.
		return nil
	}
	var sent []string

	if hasChannel(channels, NotifyChannelPanel) {
		if n.notifRepo != nil {
			_ = n.notifRepo.Create(&model.Notification{
				Level:   notifLevel(m.Level),
				Title:   m.PanelTitle,
				Content: m.PanelBody,
			})
		}
		sent = append(sent, NotifyChannelPanel)
	}
	if hasChannel(channels, NotifyChannelEmail) {
		n.sendEmail(m)
		sent = append(sent, NotifyChannelEmail)
	}
	for _, t := range []struct{ label, key, platform string }{
		{NotifyChannelWeCom, "wecom_webhook", "WeCom"},
		{NotifyChannelDingTalk, "dingtalk_webhook", "DingTalk"},
		{NotifyChannelFeishu, "feishu_webhook", "Feishu"},
	} {
		if !hasChannel(channels, t.label) {
			continue
		}
		url := n.setting(t.key)
		if url == "" {
			continue
		}
		if err := sendWebhook(n.client, url, t.platform, m.WebhookText); err != nil {
			log.Printf("%s webhook send failed: %v", t.platform, err)
			continue
		}
		sent = append(sent, t.label)
	}
	return sent
}

// setting reads one panel setting.
func (n *Notifier) setting(key string) string {
	if n.settings == nil {
		return ""
	}
	return n.settings()[key]
}

// sendEmail sends the message via SMTP when the panel has it configured.
func (n *Notifier) sendEmail(m NotifyMessage) {
	host := n.setting("smtp_host")
	port := n.setting("smtp_port")
	to := n.setting("smtp_to")
	if host == "" || port == "" || to == "" {
		return
	}
	user := n.setting("smtp_user")
	pass := n.setting("smtp_pass")
	from := n.setting("smtp_from")
	if from == "" {
		from = user
	}
	subject := m.MailSubject
	if subject == "" {
		subject = m.PanelTitle
	}
	body := "From: " + from + "\r\n" +
		"To: " + to + "\r\n" +
		"Subject: " + subject + "\r\n" +
		"Content-Type: text/plain; charset=UTF-8\r\n\r\n" +
		m.MailBody + "\r\n"
	addr := net.JoinHostPort(host, port)
	var auth smtp.Auth
	if user != "" {
		auth = smtp.PlainAuth("", user, pass, host)
	}
	if err := smtp.SendMail(addr, auth, from, []string{to}, []byte(body)); err != nil {
		log.Printf("notify email send failed: %v", err)
	}
}

// notifLevel maps a message level onto the in-panel notification level.
func notifLevel(level string) string {
	switch level {
	case "critical", "error":
		return model.NotifLevelError
	case "info", "success":
		return model.NotifLevelInfo
	default:
		return model.NotifLevelWarn
	}
}

// sendWebhook posts a text message to a group-robot webhook, adapting the payload
// per platform (Feishu uses a different envelope).
func sendWebhook(client *http.Client, url, platform, content string) error {
	var body map[string]any
	if platform == "Feishu" {
		body = map[string]any{"msg_type": "text", "content": map[string]string{"text": content}}
	} else {
		// WeCom and DingTalk share the same text format.
		body = map[string]any{"msgtype": "text", "text": map[string]string{"content": content}}
	}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	return nil
}
