package safety

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

func newTestManager(t *testing.T) (*Manager, func()) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	if err := db.AutoMigrate(&model.QuarantineItem{}, &model.SnapshotRecord{}, &model.BlockedIP{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	dir := t.TempDir()
	return New(db, func() string { return dir }, 8888), func() {}
}

func TestCheckArgProtectsCriticalPaths(t *testing.T) {
	m, done := newTestManager(t)
	defer done()

	cases := []struct {
		name string
		args map[string]any
		want string
	}{
		{"sshd config", map[string]any{"path": "/etc/ssh/sshd_config"}, "deny"},
		{"passwd", map[string]any{"path": "/etc/passwd"}, "deny"},
		{"panel dir", map[string]any{"path": "/data/opsmini/opsmini.db"}, "deny"},
		{"normal log", map[string]any{"path": "/var/log/app.log"}, "allow"},
		{"protected unit", map[string]any{"unit": "sshd"}, "deny"},
		{"rm -rf /", map[string]any{"command": "rm -rf /"}, "deny"},
		{"mkfs", map[string]any{"command": "mkfs.ext4 /dev/sdb1"}, "deny"},
		{"reboot", map[string]any{"command": "reboot"}, "deny"},
		{"plain df", map[string]any{"command": "df -h"}, "allow"},
	}
	for _, c := range cases {
		got, rule, _ := m.CheckArg("test", c.args)
		if got != c.want {
			t.Errorf("%s: got %s (%s), want %s", c.name, got, rule, c.want)
		}
	}
}

func TestKillSwitchBlocksWrites(t *testing.T) {
	m, done := newTestManager(t)
	defer done()
	m.Pause(true)
	if v, rule, _ := m.CheckArg("test", map[string]any{"path": "/var/log/app.log"}); v != "deny" || rule != "kill-switch" {
		t.Fatalf("paused manager must deny writes, got %s %s", v, rule)
	}
	m.Pause(false)
	if v, _, _ := m.CheckArg("test", map[string]any{"path": "/var/log/app.log"}); v != "allow" {
		t.Fatalf("resumed manager must allow, got %s", v)
	}
}

func TestQuarantineIsReversible(t *testing.T) {
	m, done := newTestManager(t)
	defer done()
	dir := t.TempDir()
	path := filepath.Join(dir, "shell.php")
	if err := os.WriteFile(path, []byte("<?php eval($_POST['x']);"), 0o644); err != nil {
		t.Fatal(err)
	}

	item, err := m.Quarantine("inc1", path, "test webshell")
	if err != nil {
		t.Fatalf("quarantine: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("original file should be gone")
	}
	if _, err := os.Stat(item.StoredPath); err != nil {
		t.Fatalf("stored copy missing: %v", err)
	}

	if _, err := m.RestoreQuarantine(item.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("restored file missing: %v", err)
	}
	if string(body) != "<?php eval($_POST['x']);" {
		t.Fatalf("restored content mismatch: %q", body)
	}
}

func TestSnapshotRollback(t *testing.T) {
	m, done := newTestManager(t)
	defer done()
	dir := t.TempDir()
	path := filepath.Join(dir, "app.conf")
	if err := os.WriteFile(path, []byte("keep=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	rec, err := m.Snapshot("inc1", path, "config")
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := os.WriteFile(path, []byte("broken=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := m.RestoreSnapshot(rec.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	body, _ := os.ReadFile(path)
	if string(body) != "keep=1\n" {
		t.Fatalf("rollback failed: %q", body)
	}
}

func TestWriteBudget(t *testing.T) {
	m, done := newTestManager(t)
	defer done()
	for i := 0; i < 2; i++ {
		if ok, _ := m.CountWrite("inc1", 2); !ok {
			t.Fatalf("write %d should be allowed", i+1)
		}
	}
	if ok, used := m.CountWrite("inc1", 2); ok || used != 3 {
		t.Fatalf("third write must be refused, got ok=%v used=%d", ok, used)
	}
}

func TestVerifyServiceAssertion(t *testing.T) {
	m, done := newTestManager(t)
	defer done()
	ok, detail := m.Verify(t.Context(), "inc1", []Assertion{{Type: "port_listen", Port: 1}})
	if ok {
		t.Fatalf("port 1 should not be listening: %v", detail)
	}
	ok, _ = m.Verify(t.Context(), "inc1", []Assertion{{Type: "no_new_signal", Family: "nonexistent", Window: "1m"}})
	if !ok {
		t.Fatalf("no_new_signal should pass for an unknown family")
	}
	_ = time.Now
}
