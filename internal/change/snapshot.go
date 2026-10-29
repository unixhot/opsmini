package change

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Snapshots stores pre-change copies of configuration files so a bad edit can be
// undone.
//
// Deliberate limitation: a file whose content required redaction (it contained a
// password, token or key) is NOT snapshotted. Keeping a plaintext copy of exactly
// the secrets we refuse to store would defeat the whole sanitization design.
// Such files are still recorded as changed — they just need a manual rollback.
type Snapshots struct {
	dir      string
	maxPerDay int
	maxTotal int64

	mu sync.Mutex
}

// NewSnapshots prepares the snapshot directory.
//
// If the configured directory is not writable (unprivileged run, read-only /var)
// we fall back to a temp location rather than silently dropping every snapshot:
// a feature that fails invisibly is worse than one that reports degraded
// operation. The effective directory is exposed via Dir() and shown in the panel.
func NewSnapshots(dir string, maxPerDay int, maxTotal int64) *Snapshots {
	if strings.TrimSpace(dir) == "" {
		dir = "/data/opsmini/changes"
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		fallback := filepath.Join(os.TempDir(), "opsmini-changes")
		if err2 := os.MkdirAll(fallback, 0o700); err2 == nil {
			log.Printf("change: snapshot dir %q not writable (%v), falling back to %q", dir, err, fallback)
			dir = fallback
		} else {
			log.Printf("change: snapshot dir %q unavailable, rollback will be disabled", dir)
		}
	}
	return &Snapshots{dir: dir, maxPerDay: maxPerDay, maxTotal: maxTotal}
}

// Dir returns the base directory (for display in the panel).
func (s *Snapshots) Dir() string { return s.dir }

// Save writes a pre-change copy. changeID is used as the directory name so a
// rollback is a single lookup.
func (s *Snapshots) Save(changeID, path string, content []byte) error {
	if changeID == "" || path == "" {
		return errors.New("missing id or path")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	day := time.Now().Format("2006-01-02")
	dayDir := filepath.Join(s.dir, day)
	if err := os.MkdirAll(dayDir, 0o700); err != nil {
		return err
	}
	// quota: per-day count
	if entries, err := os.ReadDir(dayDir); err == nil && s.maxPerDay > 0 && len(entries) >= s.maxPerDay {
		return errors.New("daily snapshot quota reached")
	}
	if s.maxTotal > 0 {
		if total, _ := dirSize(s.dir); total+int64(len(content)) > s.maxTotal {
			return errors.New("snapshot total quota reached")
		}
	}
	fn := filepath.Join(dayDir, changeID+".snap")
	meta := fmt.Sprintf("# path: %s\n# saved: %s\n", path, time.Now().Format(time.RFC3339))
	return os.WriteFile(fn, append([]byte(meta), content...), 0o600)
}

// Load returns the recorded pre-change content for a change id.
func (s *Snapshots) Load(changeID string) (path string, content []byte, err error) {
	matches, _ := filepath.Glob(filepath.Join(s.dir, "*", changeID+".snap"))
	if len(matches) == 0 {
		return "", nil, errors.New("snapshot not found")
	}
	raw, err := os.ReadFile(matches[0])
	if err != nil {
		return "", nil, err
	}
	// strip the two metadata header lines
	text := string(raw)
	var bodyStart int
	if i := strings.Index(text, "\n"); i >= 0 {
		if j := strings.Index(text[i+1:], "\n"); j >= 0 {
			bodyStart = i + 1 + j + 1
		}
	}
	for _, line := range strings.SplitN(text, "\n", 2) {
		if strings.HasPrefix(line, "# path: ") {
			path = strings.TrimSpace(strings.TrimPrefix(line, "# path: "))
		}
	}
	return path, []byte(text[bodyStart:]), nil
}

// Prune removes snapshots older than days.
func (s *Snapshots) Prune(days int) (int, error) {
	if days <= 0 {
		return 0, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cutoff := time.Now().AddDate(0, 0, -days)
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return 0, nil
	}
	removed := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		day, err := time.ParseInLocation("2006-01-02", e.Name(), time.Local)
		if err != nil || !day.Before(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(s.dir, e.Name()))
		removed++
	}
	return removed, nil
}

// Restore copies a snapshot back to its original path.
//
// The caller is responsible for authorisation: this function performs a write to
// the live filesystem and must only be reached from an audited, permission
// checked endpoint.
func (s *Snapshots) Restore(changeID string) (string, error) {
	path, content, err := s.Load(changeID)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("snapshot has no path")
	}
	if IsBlocked(path, nil) {
		return "", errors.New("path is in the hard-blocked list")
	}
	// Keep the current state too: if the restore makes things worse there is
	// still a way back.
	if cur, err := os.ReadFile(path); err == nil {
		_ = s.Save("pre-restore-"+time.Now().Format("20060102150405"), path, cur)
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp := path + ".opsmini-restore"
	if err := os.WriteFile(tmp, content, mode); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// List returns recent snapshot ids (for the panel's change detail page).
func (s *Snapshots) List(limit int) []string {
	if limit <= 0 {
		limit = 50
	}
	matches, _ := filepath.Glob(filepath.Join(s.dir, "*", "*.snap"))
	sort.Sort(sort.Reverse(sort.StringSlice(matches)))
	out := make([]string, 0, limit)
	for _, m := range matches {
		if len(out) >= limit {
			break
		}
		base := filepath.Base(m)
		out = append(out, strings.TrimSuffix(base, ".snap"))
	}
	return out
}

func dirSize(dir string) (int64, error) {
	var total int64
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total, err
}
