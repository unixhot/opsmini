package sentinel

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

func newResetDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	if err := db.AutoMigrate(&model.Signal{}, &model.Incident{}, &model.ActionLog{},
		&model.AgentRun{}, &model.QuarantineItem{}, &model.BlockedIP{},
		&model.TriggerPolicy{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// seedResetRows creates one row per table plus a real quarantined file so the
// reset is exercised against actual state, not just empty tables.
func seedResetRows(t *testing.T, db *gorm.DB) (quarantineFile string) {
	t.Helper()
	now := time.Now()
	db.Create(&model.Signal{ID: "sig1", Kind: "log", Severity: "high", ObservedAt: now})
	db.Create(&model.Incident{ID: "inc1", Title: "open incident", Status: model.IncidentOpen,
		Severity: "high", FirstSeen: now, LastSeen: now})
	db.Create(&model.ActionLog{Tool: "dir_usage", Risk: "L0", Verdict: "allow", CreatedAt: now})
	db.Create(&model.AgentRun{IncidentID: "inc1", Status: "success", StartedAt: now})

	dir := t.TempDir()
	quarantineFile = filepath.Join(dir, "suspicious.php")
	if err := os.WriteFile(quarantineFile, []byte("<?php ?>"), 0o600); err != nil {
		t.Fatalf("write quarantine file: %v", err)
	}
	db.Create(&model.QuarantineItem{IncidentID: "inc1", OriginalPath: "/var/www/x.php",
		StoredPath: quarantineFile, CreatedAt: now})
	db.Create(&model.BlockedIP{IP: "203.0.113.9", Source: "agent",
		ExpiresAt: now.Add(time.Hour), CreatedAt: now})

	lastRun := now.Add(-5 * time.Minute)
	db.Create(&model.TriggerPolicy{Name: "webshell-respond", Family: "webshell",
		MinSeverity: "high", Mode: "auto", LastRunAt: &lastRun})
	return quarantineFile
}

func TestResetAllWipesEveryTable(t *testing.T) {
	db := newResetDB(t)
	file := seedResetRows(t, db)
	m := &Manager{db: db}

	res, err := m.Reset("all", false)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if res.Total != 6 {
		t.Fatalf("expected 6 rows removed, got %+v", res)
	}

	for name, mdl := range map[string]any{
		"signals": &model.Signal{}, "incidents": &model.Incident{},
		"actions": &model.ActionLog{}, "runs": &model.AgentRun{},
		"quarantine": &model.QuarantineItem{}, "blocks": &model.BlockedIP{},
	} {
		var n int64
		db.Model(mdl).Count(&n)
		if n != 0 {
			t.Errorf("%s still has %d rows after reset", name, n)
		}
	}

	// The quarantined copy must go with the record, otherwise the file is left
	// orphaned in the incident directory with nothing pointing at it.
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("quarantined file was not removed: %v", err)
	}
	if res.Files != 1 {
		t.Errorf("expected 1 file removed, got %d", res.Files)
	}

	// Cooldowns are cleared so the first event after a reset is not swallowed.
	var p model.TriggerPolicy
	db.Where("name = ?", "webshell-respond").First(&p)
	if p.LastRunAt != nil {
		t.Errorf("policy cooldown not cleared: %v", p.LastRunAt)
	}
}

func TestResetEventsKeepsAuditAndBans(t *testing.T) {
	db := newResetDB(t)
	seedResetRows(t, db)
	m := &Manager{db: db}

	if _, err := m.Reset("events", false); err != nil {
		t.Fatalf("reset: %v", err)
	}

	var actions, blocks int64
	db.Model(&model.ActionLog{}).Count(&actions)
	db.Model(&model.BlockedIP{}).Count(&blocks)
	if actions != 1 || blocks != 1 {
		t.Fatalf("events scope must keep audit and bans, got actions=%d blocks=%d", actions, blocks)
	}

	var incidents int64
	db.Model(&model.Incident{}).Count(&incidents)
	if incidents != 0 {
		t.Fatalf("incidents not cleared: %d", incidents)
	}
}

func TestResetDryRunChangesNothing(t *testing.T) {
	db := newResetDB(t)
	file := seedResetRows(t, db)
	m := &Manager{db: db}

	res, err := m.Reset("all", true)
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if res.Total != 6 {
		t.Fatalf("dry run should report 6 rows, got %+v", res)
	}
	var signals int64
	db.Model(&model.Signal{}).Count(&signals)
	if signals != 1 {
		t.Fatalf("dry run deleted rows")
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("dry run removed the quarantined file: %v", err)
	}
}
