package logpipe

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreviewRingKeepsNewest(t *testing.T) {
	r := newPreviewRing(3)
	for i := 0; i < 5; i++ {
		r.add("t1", Event{"message": string(rune('a' + i))})
	}
	got := r.get("t1", 0)
	if len(got) != 3 {
		t.Fatalf("ring size=%d want 3", len(got))
	}
	if got[2].Event["message"] != "e" {
		t.Fatalf("newest=%v want e", got[2].Event["message"])
	}
}

func TestPreviewRingCopiesEvents(t *testing.T) {
	r := newPreviewRing(5)
	ev := Event{"message": "original"}
	r.add("t1", ev)
	// The pipeline hands the same map to the output next; mutating it must not
	// change what the viewer already sees.
	ev["message"] = "mutated"
	if got := r.get("t1", 0); got[0].Event["message"] != "original" {
		t.Fatalf("preview kept a reference: %v", got[0].Event["message"])
	}
}

func TestPreviewRingForget(t *testing.T) {
	r := newPreviewRing(5)
	r.add("t1", Event{"message": "x"})
	r.forget("t1")
	if got := r.get("t1", 0); len(got) != 0 {
		t.Fatalf("events survived forget: %d", len(got))
	}
}

func TestManagerPreviewAfterProcessors(t *testing.T) {
	dir := t.TempDir()
	tasks := filepath.Join(dir, "tasks.d")
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := DefaultMainConfig(dir)
	cfg.TasksDir = tasks
	m := NewManager(cfg, nil, "test")

	task := `
- type: filestream
  id: preview-task
  paths: ["` + filepath.Join(dir, "app.log") + `"]
  processors:
    - add_fields:
        target: ""
        fields:
          seen: "yes"
`
	if _, _, err := m.ApplyTask("preview.yml", []byte(task), "", false); err != nil {
		t.Fatalf("apply: %v", err)
	}
	out := m.applyProcessors("preview-task", []Event{{"message": "hello"}})
	if len(out) != 1 || out[0]["seen"] != "yes" {
		t.Fatalf("processor did not run: %v", out)
	}

	taskID, events := m.Preview("preview-task", 10)
	if taskID != "preview-task" || len(events) != 1 {
		t.Fatalf("preview returned task=%q events=%d", taskID, len(events))
	}
	if events[0].Message != "hello" {
		t.Fatalf("message=%q", events[0].Message)
	}
	// The preview must show post-processor state, which is the whole point.
	if events[0].Event["seen"] != "yes" {
		t.Fatalf("preview shows pre-processor event: %v", events[0].Event)
	}
}
