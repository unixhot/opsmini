package trigger

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/safety"
)

// fakeRunner records the prompts it was called with and returns a canned answer.
// Using it means the retry and suppression logic is tested without spending a
// single token on a real model.
type fakeRunner struct {
	calls   int
	prompts []string
	result  RunResult
	err     error
}

func (f *fakeRunner) RunIncident(ctx context.Context, req RunRequest) (RunResult, error) {
	f.calls++
	f.prompts = append(f.prompts, req.Prompt)
	r := f.result
	r.Steps, r.Tools = 3, 2
	return r, f.err
}

func newTriggerDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	if err := db.AutoMigrate(&model.Incident{}, &model.AgentRun{}, &model.TriggerPolicy{}, &model.ActionLog{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func newTestIncident(db *gorm.DB, id, group string) model.Incident {
	inc := model.Incident{
		ID: id, Title: "SSH 爆破 · 涉及 3 个源 IP", Family: "auth.anomaly",
		Subject: "sshd", Severity: "high", Status: model.IncidentOpen,
		Count: 3, MemberTotal: 3, GroupKey: group,
		FirstSeen: time.Now(), LastSeen: time.Now(),
	}
	if err := db.Create(&inc).Error; err != nil {
		panic(err)
	}
	return inc
}

// A failed verification must earn exactly one more attempt, and the retry prompt
// must carry the previous failure so the model does not repeat the same guess.
func TestRunRetriesOnceAfterVerifyFailure(t *testing.T) {
	db := newTriggerDB(t)
	inc := newTestIncident(db, "inc-retry", "auth.anomaly|bruteforce")

	runner := &fakeRunner{result: RunResult{Summary: "已排查，未发现可处置项，建议人工确认该来源 IP。"}}
	e := NewEngine(db, safety.New(db, func() string { return t.TempDir() }, 8888))
	e.Runner = runner

	p := model.TriggerPolicy{
		Name: "retry-test", Enabled: true, Family: "auth.anomaly",
		MinSeverity: "high", Mode: model.ModeAssist,
		MaxSteps: 6, MaxSeconds: 30,
		// port 1 is never listening, so the assertion fails deterministically.
		Verify: `[{"type":"port_listen","port":1}]`,
		Prompt: "排查 {title}",
	}
	e.run(inc, p)

	if runner.calls != 2 {
		t.Fatalf("expected 2 attempts after a failed verification, got %d", runner.calls)
	}
	if !strings.Contains(runner.prompts[1], "上一次处置未通过验证") {
		t.Fatalf("retry prompt should carry the previous failure, got: %s", runner.prompts[1])
	}
	if !strings.Contains(runner.prompts[1], "port_listen") {
		t.Fatalf("retry prompt should include the failed assertion detail, got: %s", runner.prompts[1])
	}

	var runs []model.AgentRun
	db.Order("id").Find(&runs)
	if len(runs) != 2 {
		t.Fatalf("expected 2 run rows (one per attempt), got %d", len(runs))
	}
	if runs[0].Attempt != 1 || runs[1].Attempt != 2 {
		t.Fatalf("attempt numbers wrong: %d, %d", runs[0].Attempt, runs[1].Attempt)
	}

	var got model.Incident
	db.First(&got, "id = ?", inc.ID)
	if got.Status != model.IncidentFailed {
		t.Fatalf("incident should be failed after both attempts, got %s", got.Status)
	}
	if got.VerifyState != "failed" {
		t.Fatalf("verify_state should be failed, got %q", got.VerifyState)
	}
}

// A group ignored by a human twice in a week must not wake the agent again, but the
// skip has to be visible as a run row instead of silently disappearing.
func TestRunSkipsSuppressedGroup(t *testing.T) {
	db := newTriggerDB(t)
	group := "auth.anomaly|bruteforce"
	for _, id := range []string{"old-1", "old-2"} {
		old := model.Incident{
			ID: id, Title: "SSH 爆破", Family: "auth.anomaly", Subject: "sshd",
			Severity: "high", Status: model.IncidentIgnored, GroupKey: group,
			FirstSeen: time.Now().Add(-2 * time.Hour), LastSeen: time.Now().Add(-time.Hour),
		}
		db.Create(&old)
		db.Model(&model.Incident{}).Where("id = ?", id).Update("updated_at", time.Now())
	}
	inc := newTestIncident(db, "inc-suppressed", group)

	runner := &fakeRunner{result: RunResult{Summary: "不该跑"}}
	e := NewEngine(db, safety.New(db, func() string { return t.TempDir() }, 8888))
	e.Runner = runner

	e.run(inc, model.TriggerPolicy{Name: "brute", Mode: model.ModeAssist, Family: "auth.anomaly"})

	if runner.calls != 0 {
		t.Fatalf("suppressed group must not call the model, got %d calls", runner.calls)
	}
	var run model.AgentRun
	if err := db.Where("incident_id = ?", inc.ID).First(&run).Error; err != nil {
		t.Fatalf("expected a skipped run row: %v", err)
	}
	if run.Status != "skipped" {
		t.Fatalf("run status should be skipped, got %s", run.Status)
	}
	if !strings.Contains(run.Error, "已被人工忽略 2 次") {
		t.Fatalf("skip reason should explain the suppression, got %q", run.Error)
	}

	// The incident stays visible and open: suppression silences the agent, not the record.
	var got model.Incident
	db.First(&got, "id = ?", inc.ID)
	if got.Status != model.IncidentOpen {
		t.Fatalf("suppressed incident should stay open, got %s", got.Status)
	}
}

// A passing verification must not retry — one call, done.
func TestRunDoesNotRetryWhenVerificationPasses(t *testing.T) {
	db := newTriggerDB(t)
	inc := newTestIncident(db, "inc-pass", "auth.anomaly|other")

	runner := &fakeRunner{result: RunResult{Summary: "已封禁来源 IP 并观察 10 分钟，无新增失败登录。"}}
	e := NewEngine(db, safety.New(db, func() string { return t.TempDir() }, 8888))
	e.Runner = runner

	e.run(inc, model.TriggerPolicy{Name: "ok", Mode: model.ModeAssist, Family: "auth.anomaly", MaxSeconds: 30})

	if runner.calls != 1 {
		t.Fatalf("a passing run must not retry, got %d calls", runner.calls)
	}
	var got model.Incident
	db.First(&got, "id = ?", inc.ID)
	if got.Status != model.IncidentResolved {
		t.Fatalf("incident should be resolved, got %s", got.Status)
	}
}
