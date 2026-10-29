package safety

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

func newJournalManager(t *testing.T) *Manager {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	if err := db.AutoMigrate(&model.ActionJournal{}, &model.QuarantineItem{},
		&model.SnapshotRecord{}, &model.BlockedIP{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	dir := t.TempDir()
	return New(db, func() string { return dir }, 8888)
}

// The core promise: a run that quarantined a file and then failed puts the file back.
func TestRollbackRestoresQuarantinedFile(t *testing.T) {
	m := newJournalManager(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "evil.php")
	if err := os.WriteFile(path, []byte("<?php system($_GET['c']); ?>"), 0o644); err != nil {
		t.Fatal(err)
	}

	item, err := m.Quarantine("sess-1", path, "test")
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("file should be gone after quarantine")
	}

	steps := m.RollbackSessions([]string{"sess-1"})
	if len(steps) != 1 || !steps[0].OK {
		t.Fatalf("expected one successful undo, got %+v", steps)
	}
	if _, err := os.Stat(item.OriginalPath); err != nil {
		t.Fatalf("file should be restored: %v", err)
	}

	// Second rollback must be a no-op, not a double restore.
	if again := m.RollbackSessions([]string{"sess-1"}); len(again) != 0 {
		t.Fatalf("rollback should be idempotent, got %+v", again)
	}
}

// Anything without an inverse must be reported, not silently dropped: that report is
// what tells a human where to look after a failed run.
func TestRollbackReportsNonUndoableWrites(t *testing.T) {
	m := newJournalManager(t)
	m.Journal("sess-2", "exec_command", "systemctl restart nginx", 0, false)

	steps := m.RollbackSessions([]string{"sess-2"})
	if len(steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(steps))
	}
	if !steps[0].Skipped || steps[0].OK {
		t.Fatalf("non-undoable write must be skipped, got %+v", steps[0])
	}
	if steps[0].Detail == "" {
		t.Fatal("skipped step needs a human-readable reason")
	}
}

// Newest first: undoing in call order would restore an older file over a newer one
// when the same path was touched twice.
func TestRollbackUndoesInReverseOrder(t *testing.T) {
	m := newJournalManager(t)
	m.Journal("sess-3", "exec_command", "first", 0, false)
	m.Journal("sess-3", "exec_command", "second", 0, false)

	steps := m.RollbackSessions([]string{"sess-3"})
	if len(steps) != 2 {
		t.Fatalf("steps = %d, want 2", len(steps))
	}
	if steps[0].Target != "second" {
		t.Fatalf("rollback order = %s first, want newest (second)", steps[0].Target)
	}
}

// A frozen process is recovered by the same rollback, because a failed run must not
// leave something suspended forever.
func TestRollbackResumesFrozenProcess(t *testing.T) {
	m := newJournalManager(t)
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start helper process: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	pid := cmd.Process.Pid
	if err := syscall.Kill(pid, syscall.SIGSTOP); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	m.Journal("sess-4", "freeze_process", strconv.Itoa(pid), 0, true)

	steps := m.RollbackSessions([]string{"sess-4"})
	if len(steps) != 1 || !steps[0].OK {
		t.Fatalf("freeze should be undone, got %+v", steps)
	}
	// SIGCONT on a stopped process must leave it alive.
	if err := syscall.Kill(pid, 0); err != nil {
		t.Fatalf("process should still exist: %v", err)
	}
}

// The journal is per session, so one incident's rollback cannot touch another's work.
func TestRollbackIsScopedToSession(t *testing.T) {
	m := newJournalManager(t)
	m.Journal("sess-a", "exec_command", "a", 0, false)
	m.Journal("sess-b", "exec_command", "b", 0, false)

	steps := m.RollbackSessions([]string{"sess-a"})
	if len(steps) != 1 || steps[0].Target != "a" {
		t.Fatalf("rollback leaked across sessions: %+v", steps)
	}
	var pending int64
	m.DB.Model(&model.ActionJournal{}).Where("session_id = ? AND undone = ?", "sess-b", false).Count(&pending)
	if pending != 1 {
		t.Fatalf("other session journal changed: pending=%d", pending)
	}
}
