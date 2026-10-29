package sentinel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

func newShadowDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	if err := db.AutoMigrate(&model.Incident{}, &model.Signal{}, &model.ActionLog{},
		&model.AgentRun{}, &model.ShadowReview{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func shadowAction(t *testing.T, db *gorm.DB, incidentID, tool string, args map[string]any, at time.Time) {
	t.Helper()
	raw, _ := json.Marshal(args)
	if err := db.Create(&model.ActionLog{
		IncidentID: incidentID, Tool: tool, Args: string(raw), Risk: "L1",
		Verdict: "shadow", Rule: "shadow-mode", Result: "影子模式：未执行", CreatedAt: at,
	}).Error; err != nil {
		t.Fatalf("seed action: %v", err)
	}
}

// A file that is still on disk is a quarantine proposal that was right — nobody else
// cleaned it up, so the agent's instinct was sound.
func TestShadowScoresQuarantineAsHelpfulWhenFileRemains(t *testing.T) {
	db := newShadowDB(t)
	now := time.Now()
	dir := t.TempDir()
	path := filepath.Join(dir, "shell.php")
	if err := os.WriteFile(path, []byte("<?php ?>"), 0o644); err != nil {
		t.Fatal(err)
	}

	db.Create(&model.Incident{ID: "inc-s1", Title: "疑似 WebShell", Family: "webshell",
		Subject: path, Severity: "high", Status: model.IncidentResolved, Mode: model.ModeShadow,
		Count: 1, FirstSeen: now.Add(-time.Hour), LastSeen: now.Add(-50 * time.Minute)})
	shadowAction(t, db, "inc-s1", "quarantine_file", map[string]any{"path": path}, now.Add(-50*time.Minute))

	rev, err := NewShadowReviewer(db).Review("inc-s1")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if rev.Verdict != model.ShadowWouldHelp {
		t.Fatalf("verdict = %s, want would_help（原因：%s）", rev.Verdict, rev.Reasons)
	}
	var inc model.Incident
	db.First(&inc, "id = ?", "inc-s1")
	if inc.ShadowVerdict != model.ShadowWouldHelp {
		t.Fatalf("incident mirror not updated: %q", inc.ShadowVerdict)
	}
}

// A proposal to block an address that then stopped knocking was not needed. This is
// the whole point of scoring: without it the operator cannot tell a useful policy
// from a noisy one.
func TestShadowScoresBlockAsUnnecessaryWhenNoRecurrence(t *testing.T) {
	db := newShadowDB(t)
	now := time.Now()
	db.Create(&model.Incident{ID: "inc-s2", Title: "SSH 爆破", Family: "auth.anomaly",
		Subject: "45.9.1.2", Severity: "high", Status: model.IncidentResolved, Mode: model.ModeShadow,
		Count: 3, FirstSeen: now.Add(-time.Hour), LastSeen: now.Add(-50 * time.Minute)})
	shadowAction(t, db, "inc-s2", "block_ip", map[string]any{"ip": "45.9.1.2"}, now.Add(-50*time.Minute))

	rev, err := NewShadowReviewer(db).Review("inc-s2")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if rev.Verdict != model.ShadowUnnecessary {
		t.Fatalf("verdict = %s, want unnecessary（原因：%s）", rev.Verdict, rev.Reasons)
	}
}

// If the same address keeps showing up afterwards, the block would have helped.
func TestShadowScoresBlockAsHelpfulOnRecurrence(t *testing.T) {
	db := newShadowDB(t)
	now := time.Now()
	db.Create(&model.Incident{ID: "inc-s3", Title: "SSH 爆破", Family: "auth.anomaly",
		Subject: "45.9.1.9", Severity: "high", Status: model.IncidentResolved, Mode: model.ModeShadow,
		Count: 3, FirstSeen: now.Add(-time.Hour), LastSeen: now.Add(-40 * time.Minute)})
	shadowAction(t, db, "inc-s3", "block_ip", map[string]any{"ip": "45.9.1.9"}, now.Add(-40*time.Minute))
	// A later signal mentioning the same address: the threat continued.
	db.Create(&model.Signal{ID: "sig-after", Kind: "auth", Family: "auth.anomaly",
		Subject: "45.9.1.9", Severity: "high", Message: "failed login from 45.9.1.9",
		ObservedAt: now.Add(-10 * time.Minute)})

	rev, err := NewShadowReviewer(db).Review("inc-s3")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if rev.Verdict != model.ShadowWouldHelp {
		t.Fatalf("verdict = %s, want would_help（原因：%s）", rev.Verdict, rev.Reasons)
	}
}

// A shadow run that only investigated has nothing to grade; saying so is better than
// inventing a verdict.
func TestShadowWithoutActionsIsUnknown(t *testing.T) {
	db := newShadowDB(t)
	now := time.Now()
	db.Create(&model.Incident{ID: "inc-s4", Title: "调查", Family: "proc.anomaly",
		Subject: "x", Severity: "high", Status: model.IncidentResolved, Mode: model.ModeShadow,
		Count: 1, FirstSeen: now, LastSeen: now})

	rev, err := NewShadowReviewer(db).Review("inc-s4")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if rev.Verdict != model.ShadowUnknown {
		t.Fatalf("verdict = %s, want unknown", rev.Verdict)
	}
}

// The batch pass must skip fresh runs (nothing has happened yet) and stop re-reviewing
// ones already scored.
func TestShadowReviewPendingRespectsAge(t *testing.T) {
	db := newShadowDB(t)
	now := time.Now()
	db.Create(&model.Incident{ID: "inc-old", Title: "a", Family: "webshell", Subject: "/tmp/a",
		Severity: "high", Status: model.IncidentResolved, Mode: model.ModeShadow,
		Count: 1, FirstSeen: now.Add(-2 * time.Hour), LastSeen: now.Add(-time.Hour)})
	db.Create(&model.Incident{ID: "inc-new", Title: "b", Family: "webshell", Subject: "/tmp/b",
		Severity: "high", Status: model.IncidentResolved, Mode: model.ModeShadow,
		Count: 1, FirstSeen: now, LastSeen: now})

	r := NewShadowReviewer(db)
	r.MinAge = 10 * time.Minute
	if n := r.ReviewPending(10); n != 1 {
		t.Fatalf("reviewed %d, want 1 (only the old one)", n)
	}
	if n := r.ReviewPending(10); n != 0 {
		t.Fatalf("second pass reviewed %d, want 0", n)
	}
}
