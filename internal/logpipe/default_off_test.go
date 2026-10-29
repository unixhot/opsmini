package logpipe

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// The single most important property of this feature: a standalone install must
// not collect anything. No goroutine, no directory scan, no memory.

func TestDefaultConfigIsOff(t *testing.T) {
	cfg := DefaultMainConfig(t.TempDir())
	if cfg.Enabled {
		t.Fatal("default config must not enable collection")
	}
	// And it must not be a task dir that already exists with files.
	m := NewManager(cfg, nil, "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	if m.Running() {
		t.Fatal("pipeline started with no task files and Enabled=false")
	}
}

func TestStartWithTaskFileTurnsOn(t *testing.T) {
	dir := t.TempDir()
	tasks := filepath.Join(dir, "tasks.d")
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		t.Fatal(err)
	}
	// A task file is itself the request to collect, even with Enabled=false.
	body := "- type: filestream\n  id: t1\n  paths: [\"" + filepath.Join(dir, "a.log") + "\"]\n"
	if err := os.WriteFile(filepath.Join(tasks, "t1.yml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultMainConfig(dir)
	cfg.TasksDir = tasks
	m := NewManager(cfg, nil, "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	if !m.Running() {
		t.Fatal("a present task file must start the collector")
	}
}

func TestApplyTaskTurnsOnAtRuntime(t *testing.T) {
	dir := t.TempDir()
	tasks := filepath.Join(dir, "tasks.d")
	cfg := DefaultMainConfig(dir)
	cfg.TasksDir = tasks
	m := NewManager(cfg, nil, "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.Start(ctx)
	if m.Running() {
		t.Fatal("precondition: should be off")
	}
	// This is the OpsAnt path: startup found nothing, then a task arrives.
	body := "- type: filestream\n  id: t2\n  paths: [\"" + filepath.Join(dir, "b.log") + "\"]\n"
	if _, _, err := m.ApplyTask("t2.yml", []byte(body), "", false); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !m.Running() {
		t.Fatal("receiving a task must start the collector")
	}
	m.Stop()
}

func TestCountTaskFilesIgnoresNonYAML(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.yml"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.yaml"), []byte("x"), 0o644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte("x"), 0o644)
	if n := countTaskFiles(dir); n != 2 {
		t.Fatalf("count=%d want 2", n)
	}
	if n := countTaskFiles(filepath.Join(dir, "missing")); n != 0 {
		t.Fatalf("missing dir count=%d", n)
	}
}
