package logpipe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:logpipe-test?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Skipf("sqlite unavailable: %v", err)
	}
	if err := db.AutoMigrate(&model.LogPipeRegistry{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

// A task written for filebeat must parse in all three shapes platforms emit.
func TestParseTaskShapes(t *testing.T) {
	bare := []byte(`
- type: filestream
  id: t1
  paths: ["/var/log/app/*.log"]
  fields:
    service: app
  processors:
    - decode_json_fields:
        fields: ["message"]
        target: ""
`)
	tasks, warns, err := ParseTask(bare, "/tmp/task-a.yml")
	if err != nil || len(tasks) != 1 {
		t.Fatalf("bare list: err=%v tasks=%d warns=%v", err, len(tasks), warns)
	}
	if tasks[0].ID != "t1" || tasks[0].Input.Fields["service"] != "app" {
		t.Fatalf("unexpected task: %+v", tasks[0])
	}
	if len(tasks[0].Input.Processors) != 1 || tasks[0].Input.Processors[0].Type != "decode_json_fields" {
		t.Fatalf("processors not parsed: %+v", tasks[0].Input.Processors)
	}

	nested := []byte(`
filebeat.inputs:
  - type: filestream
    id: t2
    paths: ["/var/log/b.log"]
`)
	if ts, _, err := ParseTask(nested, "/tmp/b.yml"); err != nil || len(ts) != 1 || ts[0].ID != "t2" {
		t.Fatalf("filebeat.inputs shape failed: %v %+v", err, ts)
	}

	flat := []byte(`
inputs:
  - type: log
    id: t3
    paths: ["/var/log/c.log"]
`)
	if ts, _, err := ParseTask(flat, "/tmp/c.yml"); err != nil || len(ts) != 1 || ts[0].ID != "t3" {
		t.Fatalf("inputs shape failed: %v %+v", err, ts)
	}
}

// Missing id must be derived (and warned about), never shared between tasks.
func TestParseTaskDerivesID(t *testing.T) {
	data := []byte("- type: filestream\n  paths: [\"/var/log/x.log\"]\n")
	ts, warns, err := ParseTask(data, "/tmp/nginx.yml")
	if err != nil || len(ts) != 1 {
		t.Fatalf("err=%v", err)
	}
	if ts[0].ID != "nginx-0" {
		t.Fatalf("derived id = %q, want nginx-0", ts[0].ID)
	}
	if len(warns) == 0 {
		t.Fatalf("expected a warning about the missing id")
	}
}

func TestMultilineAfter(t *testing.T) {
	ml := newMultiline(&Multiline{Pattern: `^\s`, Match: "after"})
	lines := []string{"2026-10-29 ERROR boom", "  at foo.go:1", "  at bar.go:2", "2026-10-29 INFO ok"}
	var out []string
	off := int64(0)
	for _, l := range lines {
		start := off
		off += int64(len(l)) + 1
		if done, ev, _, _ := ml.push(l, start, off); done {
			out = append(out, ev)
		}
	}
	if ev, ok, _, _ := ml.flush(off); ok {
		out = append(out, ev)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 events, got %d: %v", len(out), out)
	}
	if !strings.Contains(out[0], "at bar.go:2") {
		t.Fatalf("continuation not merged: %q", out[0])
	}
}

func TestMultilineBefore(t *testing.T) {
	// before-mode: the pattern marks the first line of each record.
	ml := newMultiline(&Multiline{Pattern: `^\d{4}-`, Match: "before"})
	var out []string
	off := int64(0)
	for _, l := range []string{"2026-01-01 first", "continued", "2026-01-02 second", "continued2"} {
		start := off
		off += int64(len(l)) + 1
		if done, ev, _, _ := ml.push(l, start, off); done {
			out = append(out, ev)
		}
	}
	if ev, ok, _, _ := ml.flush(off); ok {
		out = append(out, ev)
	}
	if len(out) != 2 || out[0] != "2026-01-01 first\ncontinued" {
		t.Fatalf("before-mode merge wrong: %v", out)
	}
}

// End to end: read a file, stop before the incomplete last line, keep the offset.
func TestHarvesterReadsAndTracksOffset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.log")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree"), 0o644); err != nil {
		t.Fatal(err)
	}
	reg := NewRegistry(testDB(t))
	task := Task{ID: "t", Input: Input{Type: "filestream", ID: "t", Paths: []string{path}}}
	h := &harvester{task: task, path: path, reg: reg, agent: "opsmini", version: "test"}

	batches, err := h.run(context.Background())
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	var lines []string
	var end int64
	for _, b := range batches {
		end = b.endOff
		for _, ev := range b.events {
			lines = append(lines, ev["message"].(string))
		}
	}
	// "three" has no trailing newline: it must NOT be emitted yet, otherwise a
	// writer mid-line would produce a truncated document.
	if len(lines) != 2 || lines[0] != "one" || lines[1] != "two" {
		t.Fatalf("unexpected lines: %v", lines)
	}
	if end != 8 {
		t.Fatalf("endOff = %d, want 8", end)
	}

	reg.Advance("t", path, 0, end, "")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	batches, _ = h.run(context.Background())
	var again []string
	for _, b := range batches {
		for _, ev := range b.events {
			again = append(again, ev["message"].(string))
		}
	}
	if len(again) != 1 || again[0] != "three" {
		t.Fatalf("resume should read only the new line, got %v", again)
	}
}

func TestProcessorsChain(t *testing.T) {
	procs, warns := buildProcessors([]ProcSpec{
		{Type: "decode_json_fields", Params: map[string]any{"fields": []any{"message"}, "target": "", "overwrite_keys": true}},
		{Type: "convert", Params: map[string]any{"fields": []any{map[string]any{"field": "status", "type": "long"}}}},
		{Type: "rename", Params: map[string]any{"fields": []any{map[string]any{"from": "path", "to": "url.path"}}}},
		{Type: "drop_fields", Params: map[string]any{"fields": []any{"agent", "ecs"}}},
	})
	if len(warns) != 0 {
		t.Fatalf("unexpected warnings: %v", warns)
	}
	ev := Event{"message": `{"status":"404","path":"/a"}`, "agent": Event{}, "ecs": Event{}}
	cur := []Event{ev}
	for _, p := range procs {
		var next []Event
		for _, e := range cur {
			next = append(next, p.Apply(e)...)
		}
		cur = next
	}
	if len(cur) != 1 {
		t.Fatalf("event dropped unexpectedly")
	}
	got := cur[0]
	if got["status"] != int64(404) {
		t.Fatalf("convert produced %v (%T)", got["status"], got["status"])
	}
	if _, ok := got["agent"]; ok {
		t.Fatalf("drop_fields left agent behind")
	}
	if v, ok := getPath(got, "url.path"); !ok || v != "/a" {
		t.Fatalf("rename failed: %v", got)
	}
}

func TestDropEventWhen(t *testing.T) {
	procs, _ := buildProcessors([]ProcSpec{{
		Type: "drop_event",
		Params: map[string]any{"when": map[string]any{
			"contains": map[string]any{"message": "healthcheck"},
		}},
	}})
	if got := procs[0].Apply(Event{"message": "GET /healthcheck"}); len(got) != 0 {
		t.Fatalf("event should be dropped")
	}
	if got := procs[0].Apply(Event{"message": "GET /api"}); len(got) != 1 {
		t.Fatalf("event should be kept")
	}
}

// The ES output must produce NDJSON with the date expanded from @timestamp and
// must treat a 200 with item errors as a partial failure.
func TestESOutputBulk(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		if r.Header.Get("Content-Type") != "application/x-ndjson" {
			t.Errorf("wrong content type: %s", r.Header.Get("Content-Type"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errors":false,"items":[]}`))
	}))
	defer srv.Close()

	out, err := newESOutput(OutputSpec{Type: "elasticsearch", Hosts: []string{srv.URL}, Index: "logs-%{+yyyy.MM.dd}"})
	if err != nil {
		t.Fatal(err)
	}
	if err := out.Publish(context.Background(), []Event{
		{"@timestamp": "2026-10-29T01:02:03Z", "message": "a"},
		{"@timestamp": "2026-10-28T01:02:03Z", "message": "b"},
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("expected one bulk request, got %d", len(bodies))
	}
	lines := strings.Split(strings.TrimSpace(bodies[0]), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 NDJSON lines, got %d: %s", len(lines), bodies[0])
	}
	var meta map[string]map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &meta); err != nil {
		t.Fatal(err)
	}
	if meta["index"]["_index"] != "logs-2026.10.29" {
		t.Fatalf("index not expanded from @timestamp: %v", meta)
	}
	if out.Stats().Sent != 2 {
		t.Fatalf("sent counter = %d", out.Stats().Sent)
	}
}

// No tasks.d and no output must leave the pipeline inert instead of erroring.
func TestManagerIdleWithoutTasks(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultMainConfig(dir)
	cfg.Output = OutputSpec{Type: "elasticsearch", Hosts: []string{"http://127.0.0.1:1"}, Index: "x"}
	m := NewManager(cfg, testDB(t), "test")
	st := m.Status()
	if len(st.Tasks) != 0 {
		t.Fatalf("expected no tasks, got %d", len(st.Tasks))
	}
	if !strings.Contains(m.Metrics(), "opsmini_log_pipeline_tasks 0") {
		t.Fatalf("metrics missing task gauge: %s", m.Metrics())
	}
}
