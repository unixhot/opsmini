package service

import (
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// newAlertTestMonitor builds a monitor against an in-memory database. The
// notifier has no repository and the rule has no channels, so Send() is a no-op:
// these tests are about what gets recorded, not about delivery.
func newAlertTestMonitor(t *testing.T, window time.Duration) (*AlertMonitor, *repository.AlertEventRepo) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&model.AlertEvent{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	repo := repository.NewAlertEventRepo(db)
	m := &AlertMonitor{
		eventRepo: repo,
		notifier:  NewNotifier(nil, func() map[string]string { return nil }),
		notifyWindow: func() time.Duration {
			return window
		},
	}
	return m, repo
}

func testRule() model.AlertRule {
	return model.AlertRule{ID: 7, Name: "磁盘使用率过高", Metric: "磁盘使用率", Notify: ""}
}

func countEvents(t *testing.T, repo *repository.AlertEventRepo) []model.AlertEvent {
	t.Helper()
	all, err := repo.List(100)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	return all
}

// resolveAll marks every active event resolved, simulating a recovery. The next
// breach then looks exactly like a real flap: active check passes, but the
// notification window should still hold.
func resolveAll(t *testing.T, repo *repository.AlertEventRepo) {
	t.Helper()
	rows := countEvents(t, repo)
	for i := range rows {
		if rows[i].Status != model.AlertStatusActive {
			continue
		}
		rows[i].Status = model.AlertStatusResolved
		now := time.Now()
		rows[i].ResolvedAt = &now
		if err := repo.Update(&rows[i]); err != nil {
			t.Fatalf("resolve: %v", err)
		}
	}
}

// A flap — breach, recovery, breach — must produce two events but only one
// notification. Recording both keeps the timeline honest; notifying both is what
// makes people mute the channel.
func TestAlertSuppressionSilencesFlap(t *testing.T) {
	m, repo := newAlertTestMonitor(t, 30*time.Minute)
	rule := testRule()

	m.fire(rule, 95, 90, ">")
	resolveAll(t, repo)
	m.fire(rule, 95, 90, ">")

	events := countEvents(t, repo)
	if len(events) != 2 {
		t.Fatalf("events: got %d want 2", len(events))
	}
	// Newest first from List.
	var notified, silenced int
	for _, e := range events {
		if e.NotifiedAt != nil {
			notified++
		} else {
			silenced++
		}
	}
	if notified != 1 || silenced != 1 {
		t.Fatalf("notified=%d silenced=%d, want 1/1", notified, silenced)
	}
	if !strings.Contains(events[0].Message, "静默") {
		t.Fatalf("silenced event should say so, got %q", events[0].Message)
	}
}

// window=0 disables suppression: an operator who wants every recurrence to ring
// must get every recurrence.
func TestAlertSuppressionDisabled(t *testing.T) {
	m, repo := newAlertTestMonitor(t, 0)
	rule := testRule()

	m.fire(rule, 95, 90, ">")
	resolveAll(t, repo)
	m.fire(rule, 95, 90, ">")

	events := countEvents(t, repo)
	if len(events) != 2 {
		t.Fatalf("events: got %d want 2", len(events))
	}
	for _, e := range events {
		if e.NotifiedAt == nil {
			t.Fatalf("window=0 must notify every time, event %d was silenced", e.ID)
		}
	}
}

// An event that is still active is not recreated at all — that is the older,
// narrower dedup rule and it must keep working alongside the new one.
func TestAlertActiveEventNotDuplicated(t *testing.T) {
	m, repo := newAlertTestMonitor(t, 30*time.Minute)
	rule := testRule()

	m.fire(rule, 95, 90, ">")
	m.fire(rule, 96, 90, ">")
	m.fire(rule, 97, 90, ">")

	if n := len(countEvents(t, repo)); n != 1 {
		t.Fatalf("events: got %d want 1", n)
	}
}

// Different rules must not silence each other: the window is per rule.
func TestAlertSuppressionIsPerRule(t *testing.T) {
	m, repo := newAlertTestMonitor(t, 30*time.Minute)
	a := testRule()
	b := model.AlertRule{ID: 8, Name: "内存使用率过高", Metric: "内存使用率", Notify: ""}

	m.fire(a, 95, 90, ">")
	resolveAll(t, repo)
	m.fire(b, 95, 90, ">")

	events := countEvents(t, repo)
	if len(events) != 2 {
		t.Fatalf("events: got %d want 2", len(events))
	}
	for _, e := range events {
		if e.NotifiedAt == nil {
			t.Fatalf("rule %s should have notified: it is a different rule", e.RuleName)
		}
	}
}

func TestHumanWindow(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Minute, "30 分钟"},
		{90 * time.Minute, "90 分钟"},
		{2 * time.Hour, "2 小时"},
		{45 * time.Second, "45 秒"},
	}
	for _, c := range cases {
		if got := humanWindow(c.d); got != c.want {
			t.Fatalf("humanWindow(%v): got %q want %q", c.d, got, c.want)
		}
	}
}
