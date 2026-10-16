package service

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// testDB opens an in-memory SQLite database with the notification table.
func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Notification{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// TestHasChannel verifies channel matching for the labels used by the panel and
// the English aliases kept for backwards compatibility.
func TestHasChannel(t *testing.T) {
	cases := []struct {
		channels string
		label    string
		want     bool
	}{
		{"面板", NotifyChannelPanel, true},
		{"面板,邮件", NotifyChannelEmail, true},
		{"面板,邮件", NotifyChannelWeCom, false},
		{"站内", NotifyChannelPanel, true},
		{"email", NotifyChannelEmail, true},
		{"企业微信", NotifyChannelWeCom, true},
		{"钉钉", NotifyChannelDingTalk, true},
		{"飞书", NotifyChannelFeishu, true},
		{"", NotifyChannelPanel, false},
		{"邮件", NotifyChannelDingTalk, false},
	}
	for _, c := range cases {
		if got := hasChannel(c.channels, c.label); got != c.want {
			t.Errorf("hasChannel(%q, %q) = %v, want %v", c.channels, c.label, got, c.want)
		}
	}
}

// TestNotifierEmptyChannels verifies that an empty channel list delivers nothing:
// alerts with no channel configured must not create unexpected notifications.
func TestNotifierEmptyChannels(t *testing.T) {
	n := NewNotifier(nil, func() map[string]string { return map[string]string{} })
	if sent := n.Send(NotifyMessage{Channels: "", PanelTitle: "x"}); len(sent) != 0 {
		t.Errorf("empty channel list must deliver nothing, got %v", sent)
	}
	// A nil notifier is a no-op, never a panic.
	var nilNotifier *Notifier
	if sent := nilNotifier.Send(NotifyMessage{Channels: NotifyChannelPanel}); sent != nil {
		t.Errorf("nil notifier must be a no-op, got %v", sent)
	}
}

// TestNotifierPanelChannel verifies the in-panel channel actually writes a
// notification through the repository.
func TestNotifierPanelChannel(t *testing.T) {
	db := testDB(t)
	repo := repository.NewNotificationRepo(db)
	n := NewNotifier(repo, func() map[string]string { return map[string]string{} })

	sent := n.Send(NotifyMessage{
		Channels:   NotifyChannelPanel,
		Level:      "info",
		PanelTitle: "AI 任务「每日巡检」执行完成",
		PanelBody:  "共 6 步",
	})
	if len(sent) != 1 || sent[0] != NotifyChannelPanel {
		t.Fatalf("unexpected channels: %v", sent)
	}
	list, err := repo.List(10)
	if err != nil {
		t.Fatalf("list notifications: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("want 1 notification, got %d", len(list))
	}
	if !strings.Contains(list[0].Title, "每日巡检") {
		t.Errorf("unexpected title: %s", list[0].Title)
	}
	if list[0].Level != model.NotifLevelInfo {
		t.Errorf("level: want %s, got %s", model.NotifLevelInfo, list[0].Level)
	}
}

// TestNotifLevel verifies the level mapping into the in-panel notification levels.
func TestNotifLevel(t *testing.T) {
	cases := map[string]string{
		"critical": model.NotifLevelError,
		"error":    model.NotifLevelError,
		"warning":  model.NotifLevelWarn,
		"info":     model.NotifLevelInfo,
		"success":  model.NotifLevelInfo,
		"":         model.NotifLevelWarn,
	}
	for in, want := range cases {
		if got := notifLevel(in); got != want {
			t.Errorf("notifLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCronReqValidate verifies that the mandatory field depends on the task kind.
func TestCronReqValidate(t *testing.T) {
	cases := []struct {
		name    string
		req     CronReq
		wantErr bool
	}{
		{"面板任务带命令", CronReq{Kind: model.CronKindPanel, Schedule: "0 3 * * *", Command: "echo hi"}, false},
		{"面板任务缺命令", CronReq{Kind: model.CronKindPanel, Schedule: "0 3 * * *"}, true},
		{"Agent 任务带提示词", CronReq{Kind: model.CronKindAgent, Schedule: "0 8 * * *", Prompt: "巡检主机"}, false},
		{"Agent 任务缺提示词", CronReq{Kind: model.CronKindAgent, Schedule: "0 8 * * *"}, true},
		{"Agent 任务空提示词（仅空格）", CronReq{Kind: model.CronKindAgent, Schedule: "0 8 * * *", Prompt: "   "}, true},
		{"缺 cron 表达式", CronReq{Kind: model.CronKindAgent, Prompt: "巡检"}, true},
		{"系统任务不可创建", CronReq{Kind: model.CronKindSystem, Schedule: "* * * * *", Command: "x"}, true},
		{"用户任务不可创建", CronReq{Kind: model.CronKindUser, Schedule: "* * * * *", Command: "x"}, true},
	}
	for _, c := range cases {
		err := c.req.validate()
		if c.wantErr && err == nil {
			t.Errorf("%s: expected an error", c.name)
		}
		if !c.wantErr && err != nil {
			t.Errorf("%s: unexpected error %v", c.name, err)
		}
	}
}

// TestCronReqApplyAgentFields verifies the AI task fields land on the model.
func TestCronReqApplyAgentFields(t *testing.T) {
	req := CronReq{
		Kind: model.CronKindAgent, Schedule: "0 8 * * *", Prompt: "巡检",
		Notify: "面板,邮件", MaxSteps: 8, TimeoutSeconds: 240,
	}
	var j model.CronJob
	req.applyAgentFields(&j)
	if j.Prompt != "巡检" || j.Notify != "面板,邮件" || j.MaxSteps != 8 || j.TimeoutSeconds != 240 {
		t.Fatalf("agent fields not applied: %+v", j)
	}
}

// TestAgentFailLimit documents the auto-disable threshold.
func TestAgentFailLimit(t *testing.T) {
	if agentFailLimit != 3 {
		t.Errorf("auto-disable threshold changed: %d", agentFailLimit)
	}
}
