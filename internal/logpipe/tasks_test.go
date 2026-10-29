package logpipe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTaskFileValid(t *testing.T) {
	ok := []string{"task-1024-nginx.yml", "a.yaml"}
	for _, n := range ok {
		if err := TaskFileValid(n); err != nil {
			t.Fatalf("%s should be valid: %v", n, err)
		}
	}
	// Any of these would let a remote caller write outside tasks.d or drop a file
	// the loader ignores, which looks like success but collects nothing.
	bad := []string{"", "../evil.yml", "sub/x.yml", "/etc/passwd", ".hidden.yml", "task.txt", "a/b/../c.yml"}
	for _, n := range bad {
		if err := TaskFileValid(n); err == nil {
			t.Fatalf("%s should be rejected", n)
		}
	}
}

func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	tasks := filepath.Join(dir, "tasks.d")
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := DefaultMainConfig(dir)
	cfg.TasksDir = tasks
	cfg.Enabled = true
	m := NewManager(cfg, nil, "test")
	return m, tasks
}

const sampleTask = `
- type: filestream
  id: task-1024-nginx
  paths: ["/var/log/nginx/access.log"]
  fields:
    task_id: "1024"
`

func TestApplyTaskWritesAndLoads(t *testing.T) {
	m, tasks := newTestManager(t)
	status, _, err := m.ApplyTask("task-1024.yml", []byte(sampleTask), "", false)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if status != "applied" {
		t.Fatalf("status=%s want applied", status)
	}
	if _, err := os.Stat(filepath.Join(tasks, "task-1024.yml")); err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if _, ok := m.TaskByFile("task-1024.yml"); !ok {
		t.Fatal("task should be loaded after apply")
	}
}

func TestApplyTaskIsIdempotent(t *testing.T) {
	m, _ := newTestManager(t)
	if _, _, err := m.ApplyTask("t.yml", []byte(sampleTask), "", false); err != nil {
		t.Fatalf("first apply: %v", err)
	}
	// Same content must not rewrite: a rewrite changes the hash and restarts the
	// harvester, which loses the multiline buffer and reopens every file.
	status, _, err := m.ApplyTask("t.yml", []byte(sampleTask), "", false)
	if err != nil {
		t.Fatalf("second apply: %v", err)
	}
	if status != "unchanged" {
		t.Fatalf("status=%s want unchanged", status)
	}
}

func TestApplyTaskRejectsBadDigest(t *testing.T) {
	m, tasks := newTestManager(t)
	if _, _, err := m.ApplyTask("t.yml", []byte(sampleTask), "deadbeef", false); err == nil {
		t.Fatal("digest mismatch must fail")
	}
	if _, err := os.Stat(filepath.Join(tasks, "t.yml")); !os.IsNotExist(err) {
		t.Fatal("a rejected task must not be written")
	}
}

func TestApplyTaskRejectsInvalidYAMLAndKeepsPrevious(t *testing.T) {
	m, tasks := newTestManager(t)
	if _, _, err := m.ApplyTask("t.yml", []byte(sampleTask), "", false); err != nil {
		t.Fatalf("initial apply: %v", err)
	}
	// A broken dispatch must not take collection down: the previous version has to
	// survive so the host keeps shipping logs.
	status, _, err := m.ApplyTask("t.yml", []byte("this is: [not, valid, task"), "", false)
	if err == nil {
		t.Fatal("invalid content must fail")
	}
	if status != "failed" {
		t.Fatalf("status=%s want failed", status)
	}
	body, _ := os.ReadFile(filepath.Join(tasks, "t.yml"))
	if string(body) != sampleTask {
		t.Fatal("previous task file must be preserved")
	}
	if _, ok := m.TaskByFile("t.yml"); !ok {
		t.Fatal("previous task must still be loaded")
	}
}

func TestRemoveTaskMissingIsNotAnError(t *testing.T) {
	m, _ := newTestManager(t)
	// Dispatch may be retried after a network failure, so removing an absent task
	// must succeed rather than report a failure the platform would keep retrying.
	if err := m.RemoveTask("nope.yml"); err != nil {
		t.Fatalf("removing a missing task: %v", err)
	}
	if err := m.RemoveTask("../evil.yml"); err == nil {
		t.Fatal("path traversal must be rejected")
	}
}
