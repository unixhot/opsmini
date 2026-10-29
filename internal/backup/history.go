package backup

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// RunRecord is one entry of the backup history.
//
// Snapshots on disk only prove that successful runs happened — a run that failed
// half way leaves nothing behind, which is exactly the case an operator needs to
// be told about. The history is therefore a separate append-only file.
type RunRecord struct {
	Job       string    `json:"job"`
	Snapshot  string    `json:"snapshot,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	OK        bool      `json:"ok"`
	Error     string    `json:"error,omitempty"`
	Size      int64     `json:"size,omitempty"`
	Objects   int       `json:"objects,omitempty"`
}

// historyLimit bounds the file so a frequent schedule cannot grow it forever.
const historyLimit = 2000

var historyMu sync.Mutex

func (m *Manager) historyPath() string { return filepath.Join(m.root, "history.jsonl") }

// record appends one run to the history file and trims it to historyLimit lines.
func (m *Manager) record(rec RunRecord) {
	historyMu.Lock()
	defer historyMu.Unlock()

	line, err := json.Marshal(rec)
	if err != nil {
		return
	}
	f, err := os.OpenFile(m.historyPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return
	}
	f.Close()
	m.trimHistory()
}

// trimHistory keeps the newest historyLimit records.
func (m *Manager) trimHistory() {
	data, err := os.ReadFile(m.historyPath())
	if err != nil {
		return
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) <= historyLimit {
		return
	}
	kept := lines[len(lines)-historyLimit:]
	tmp := m.historyPath() + ".tmp"
	if err := os.WriteFile(tmp, []byte(strings.Join(kept, "\n")+"\n"), 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, m.historyPath())
}

// History returns the recorded runs, newest first. A limit of 0 means "all".
func (m *Manager) History(limit int) []RunRecord {
	historyMu.Lock()
	defer historyMu.Unlock()

	f, err := os.Open(m.historyPath())
	if err != nil {
		return nil
	}
	defer f.Close()

	out := make([]RunRecord, 0, 64)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var rec RunRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		out = append(out, rec)
	}
	// Newest first: the panel and the alert check both want recent runs.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Failures counts failed runs that finished inside the window. It also reports the
// most recent error so the alert message can say what broke.
func (m *Manager) Failures(window time.Duration) (int, string, bool) {
	cut := time.Now().Add(-window)
	n := 0
	last := ""
	for _, rec := range m.History(0) {
		if rec.EndedAt.Before(cut) {
			continue
		}
		if rec.OK {
			continue
		}
		n++
		if last == "" {
			last = rec.Job + ": " + rec.Error
		}
	}
	return n, last, true
}
