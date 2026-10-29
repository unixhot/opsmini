package logpipe

import (
	"sync"
	"time"
)

// Live preview.
//
// filebeat's biggest operational gap is that you cannot see what a pipeline
// actually produces. `test output` proves the endpoint is reachable; it says
// nothing about whether your dissect tokenizer matched, whether the multiline
// pattern merged the stack trace, or whether drop_fields removed the field you
// needed. Debugging that today means sending test logs to the real index.
//
// So the pipeline keeps the last few processed events per task in memory. Events
// are copied before storing: the maps are handed to the output next and must not
// change underneath the viewer.

// previewLimit is how many events per task are retained. Small on purpose: this
// is a debugging aid, not a buffer.
const previewLimit = 50

type previewEntry struct {
	At    time.Time
	Event Event
}

type previewRing struct {
	mu   sync.Mutex
	max  int
	data map[string][]previewEntry
}

func newPreviewRing(max int) *previewRing {
	return &previewRing{max: max, data: map[string][]previewEntry{}}
}

// add appends one event, dropping the oldest when full.
func (r *previewRing) add(taskID string, ev Event) {
	copied := make(Event, len(ev))
	for k, v := range ev {
		copied[k] = v
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	buf := r.data[taskID]
	buf = append(buf, previewEntry{At: time.Now(), Event: copied})
	if len(buf) > r.max {
		buf = buf[len(buf)-r.max:]
	}
	r.data[taskID] = buf
}

// get returns the retained events, newest last.
func (r *previewRing) get(taskID string, limit int) []previewEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	buf := r.data[taskID]
	if limit > 0 && len(buf) > limit {
		buf = buf[len(buf)-limit:]
	}
	out := make([]previewEntry, len(buf))
	copy(out, buf)
	return out
}

// forget drops a task's events when it disappears.
func (r *previewRing) forget(taskID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.data, taskID)
}

// PreviewEvent is one retained event as the panel sees it.
type PreviewEvent struct {
	At      time.Time `json:"at"`
	Event   Event     `json:"event"`
	Message string    `json:"message,omitempty"`
}

// Preview returns the most recent processed events of one task.
//
// If taskID is empty the busiest task is used, which is what the panel wants when
// the operator has not picked one yet.
func (m *Manager) Preview(taskID string, limit int) (string, []PreviewEvent) {
	if limit <= 0 || limit > previewLimit {
		limit = 20
	}
	if taskID == "" {
		m.mu.RLock()
		for id := range m.tasks {
			taskID = id
			break
		}
		m.mu.RUnlock()
	}
	if taskID == "" {
		return "", nil
	}
	entries := m.pre.get(taskID, limit)
	out := make([]PreviewEvent, 0, len(entries))
	for _, e := range entries {
		msg := ""
		if s, ok := e.Event["message"].(string); ok {
			msg = s
		}
		out = append(out, PreviewEvent{At: e.At, Event: e.Event, Message: msg})
	}
	return taskID, out
}
