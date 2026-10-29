package logpipe

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// spool is the disk queue in front of the output.
//
// When the destination is down for longer than the in-memory retries cover, the
// batch is written here and the file offset still advances — the data is on disk,
// which is strictly safer than holding it in RAM. A crashed process replays the
// spool on the next start.
type spool struct {
	dir     string
	maxSize int64

	mu       sync.Mutex
	written  int64
	replayed int64
	dropped  int64
	lastErr  string
}

func newSpool(dir string, maxMB int) *spool {
	if maxMB <= 0 {
		maxMB = 512
	}
	_ = os.MkdirAll(dir, 0o755)
	return &spool{dir: dir, maxSize: int64(maxMB) << 20}
}

// Write persists one batch. The atomically renamed file is the unit of recovery:
// a half-written spool file is never picked up.
func (s *spool) Write(events []Event) error {
	if len(events) == 0 {
		return nil
	}
	body, err := json.Marshal(events)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sizeLocked() > s.maxSize {
		// Oldest first: dropping the tail would lose the newest evidence, and the
		// oldest is what a long outage makes least useful.
		if name := s.oldestLocked(); name != "" {
			if os.Remove(name) == nil {
				atomic.AddInt64(&s.dropped, 1)
			}
		}
	}
	name := filepath.Join(s.dir, fmt.Sprintf("%020d.json", time.Now().UnixNano()))
	tmp := name + ".tmp"
	if err := os.WriteFile(tmp, body, 0o600); err != nil {
		s.lastErr = err.Error()
		return err
	}
	if err := os.Rename(tmp, name); err != nil {
		s.lastErr = err.Error()
		return err
	}
	atomic.AddInt64(&s.written, 1)
	return nil
}

// Replay sends the spooled batches in order and deletes each one on success.
func (s *spool) Replay(ctx context.Context, publish func([]Event) error) int {
	files := s.files()
	n := 0
	for _, f := range files {
		select {
		case <-ctx.Done():
			return n
		default:
		}
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var events []Event
		if err := json.Unmarshal(data, &events); err != nil {
			// Corrupt file: keep it out of the way instead of replaying forever.
			_ = os.Rename(f, f+".bad")
			continue
		}
		if err := publish(events); err != nil {
			s.mu.Lock()
			s.lastErr = err.Error()
			s.mu.Unlock()
			return n
		}
		if err := os.Remove(f); err == nil {
			n++
			atomic.AddInt64(&s.replayed, 1)
		}
	}
	return n
}

func (s *spool) files() []string {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		out = append(out, filepath.Join(s.dir, e.Name()))
	}
	sort.Strings(out)
	return out
}

func (s *spool) sizeLocked() int64 {
	var total int64
	for _, f := range s.files() {
		if st, err := os.Stat(f); err == nil {
			total += st.Size()
		}
	}
	return total
}

func (s *spool) oldestLocked() string {
	files := s.files()
	if len(files) == 0 {
		return ""
	}
	return files[0]
}

// Stats reports the backlog for the panel and metrics.
func (s *spool) Stats() map[string]any {
	files := s.files()
	var size int64
	for _, f := range files {
		if st, err := os.Stat(f); err == nil {
			size += st.Size()
		}
	}
	s.mu.Lock()
	lastErr := s.lastErr
	s.mu.Unlock()
	return map[string]any{
		"pending":    len(files),
		"bytes":      size,
		"written":    atomic.LoadInt64(&s.written),
		"replayed":   atomic.LoadInt64(&s.replayed),
		"dropped":    atomic.LoadInt64(&s.dropped),
		"last_error": lastErr,
	}
}

// Pending reports whether anything is waiting to be replayed.
func (s *spool) Pending() int {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	return n
}

var _ = log.Printf
var _ = strconv.Itoa
