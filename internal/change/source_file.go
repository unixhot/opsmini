package change

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/opsmini/opsmini/internal/model"
)

// maxTrackedFileSize bounds what we are willing to read. Bigger files only get
// a hash, not a diff — reading a 100MB "config" into memory would be a self
// inflicted outage.
const maxTrackedFileSize = 2 * 1024 * 1024

// maxCachedBytes bounds the total in-memory "before" content. Only sanitized
// content is cached, never plaintext, so a memory dump does not leak secrets.
const maxCachedBytes = 64 * 1024 * 1024

// maxWatchedDirs bounds inotify usage. Watching every inode under /etc exhausts
// max_user_watches (8192 by default) and steals quota from systemd and friends,
// so we watch directories only and cap the count.
const maxWatchedDirs = 300

type fileEntry struct {
	rawHash   string // hash of the on-disk bytes, detects "even the secret changed"
	clean     string // sanitized content, used for diffing
	hasSecret bool   // redaction happened: never keep a plaintext snapshot of this file
	size      int64
	mtime     time.Time
}

type fileSource struct {
	cfg   *Config
	store *Store
	snap  *Snapshots
	// persist writes synchronously and returns the final row id (which may be an
	// existing id when the change was deduplicated). The snapshot must be filed
	// under that id, otherwise rollback cannot find it.
	persist func(Event) string

	mu      sync.Mutex
	cache   map[string]*fileEntry
	cached  int64
	pending map[string]*time.Timer

	watcher   *fsnotify.Watcher
	watchDirs map[string]struct{}
	rtOK      bool
}

func newFileSource(cfg *Config, store *Store, snap *Snapshots, persist func(Event) string) *fileSource {
	return &fileSource{
		cfg:       cfg,
		store:     store,
		snap:      snap,
		persist:   persist,
		cache:     make(map[string]*fileEntry),
		pending:   make(map[string]*time.Timer),
		watchDirs: make(map[string]struct{}),
	}
}

func (s *fileSource) Name() string { return "file" }

// Run starts both channels: the realtime watcher (fast, may miss events) and the
// periodic indexer (slow, catches everything). Neither is allowed to take the
// other down.
func (s *fileSource) Run(ctx context.Context) {
	// Baseline scan: build the index WITHOUT recording anything.
	// Recording 500 "first seen" rows on every start would flood the timeline with
	// non-events and bury the one change that actually matters.
	s.scanAll(true)

	if s.cfg.File.Realtime {
		go s.runRealtime(ctx)
	}
	go s.runPeriodic(ctx)
	<-ctx.Done()
	s.stop()
}

// --- realtime channel ------------------------------------------------------

func (s *fileSource) runRealtime(ctx context.Context) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("change: fsnotify unavailable, falling back to periodic scan only: %v", err)
		return
	}
	s.watcher = w
	s.rtOK = true
	defer func() {
		s.rtOK = false
		_ = w.Close()
	}()

	for _, dir := range s.cfg.File.WatchDirs {
		s.addWatchRecursive(dir, 0)
	}
	log.Printf("change: realtime channel watching %d dirs", len(s.watchDirs))

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			if ev.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			// A newly created directory needs its own watch, otherwise every
			// file created inside it afterwards is invisible.
			if ev.Op&fsnotify.Create != 0 {
				if st, err := os.Stat(ev.Name); err == nil && st.IsDir() {
					s.addWatchRecursive(ev.Name, 0)
					continue
				}
			}
			s.schedule(ev.Name)
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			log.Printf("change: fsnotify error: %v", err)
		}
	}
}

func (s *fileSource) addWatchRecursive(dir string, depth int) {
	if depth > 4 || len(s.watchDirs) >= maxWatchedDirs {
		return
	}
	dir = filepath.Clean(dir)
	if _, ok := s.watchDirs[dir]; ok {
		return
	}
	if IsBlocked(dir+"/x", s.cfg.File.ExtraBlocked) {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	if s.watcher != nil {
		if err := s.watcher.Add(dir); err != nil {
			return
		}
	}
	s.watchDirs[dir] = struct{}{}
	for _, e := range entries {
		if e.IsDir() {
			s.addWatchRecursive(filepath.Join(dir, e.Name()), depth+1)
		}
	}
}

// schedule debounces path events. Editors write a file several times (and vim
// writes a temp file then renames it), so a 200ms window collapses one save
// into one change.
func (s *fileSource) schedule(path string) {
	if !s.accept(path) {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if t, ok := s.pending[path]; ok {
		t.Reset(200 * time.Millisecond)
		return
	}
	s.pending[path] = time.AfterFunc(200*time.Millisecond, func() {
		s.mu.Lock()
		delete(s.pending, path)
		s.mu.Unlock()
		s.handle(path, "realtime", false)
	})
}

// accept applies the path filters (blocklist, ignores, extension noise).
func (s *fileSource) accept(path string) bool {
	if path == "" {
		return false
	}
	p := filepath.Clean(path)
	if st, err := os.Stat(p); err == nil && st.IsDir() {
		return false
	}
	if IsBlocked(p, s.cfg.File.ExtraBlocked) {
		return false
	}
	base := filepath.Base(p)
	// editor and OS noise
	for _, suffix := range []string{"~", ".swp", ".swx", ".tmp", ".bak", ".rpmnew", ".rpmsave", ".dpkg-new", ".dpkg-old", ".lock"} {
		if strings.HasSuffix(base, suffix) {
			return false
		}
	}
	if strings.HasPrefix(base, ".#") || strings.HasPrefix(base, ".") && strings.HasSuffix(base, ".swp") {
		return false
	}
	if !s.withinWatchDirs(p) {
		return false
	}
	for _, pat := range append(s.cfg.File.IgnoreGlobs, s.userIgnores()...) {
		// matchGlob, not filepath.Match: user rules are written with "**" and must
		// behave the way people expect from rsync/gitignore.
		if matchGlob(pat, p) {
			return false
		}
	}
	return true
}

func (s *fileSource) userIgnores() []string { return s.store.IgnorePatterns() }

func (s *fileSource) withinWatchDirs(p string) bool {
	for _, d := range s.cfg.File.WatchDirs {
		if strings.HasPrefix(p, filepath.Clean(d)+string(os.PathSeparator)) || p == filepath.Clean(d) {
			return true
		}
	}
	return false
}

// --- periodic channel ------------------------------------------------------

func (s *fileSource) runPeriodic(ctx context.Context) {
	interval := s.cfg.File.ScanInterval
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.scanAll(false)
		}
	}
}

// scanAll walks the watch dirs and compares (size, mtime) against the cache.
// stat only — no content is read unless something actually changed.
func (s *fileSource) scanAll(baseline bool) {
	for _, root := range s.cfg.File.WatchDirs {
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			if !s.accept(path) {
				return nil
			}
			s.mu.Lock()
			prev, ok := s.cache[path]
			s.mu.Unlock()
			if ok && prev.size == info.Size() && prev.mtime.Equal(info.ModTime()) {
				return nil
			}
			s.handle(path, "scan", baseline)
			return nil
		})
	}
}

// --- core: read, sanitize, diff, emit --------------------------------------

func (s *fileSource) handle(path, via string, baseline bool) {
	st, err := os.Stat(path)
	if err != nil {
		// File removed: still a change worth recording.
		s.mu.Lock()
		prev, had := s.cache[path]
		if had {
			delete(s.cache, path)
			s.cached -= int64(len(prev.clean))
		}
		s.mu.Unlock()
		if had {
			ev := NewEvent(model.ChangeKindConfig, "change.file", Object{Type: "file", Key: path}, time.Now())
			ev.Diff = &Diff{Changed: true, HashFrom: prev.rawHash, Body: "(文件已删除)\n"}
			ev.Actor = s.inferActor()
			s.persist(ev)
		}
		return
	}
	if st.Size() > maxTrackedFileSize {
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	rawSum := sha256.Sum256(raw)
	rawHash := hex.EncodeToString(rawSum[:])

	s.mu.Lock()
	prev, had := s.cache[path]
	if had && prev.rawHash == rawHash {
		prev.size, prev.mtime = st.Size(), st.ModTime()
		s.mu.Unlock()
		return // touched but content identical
	}
	s.mu.Unlock()

	// Sanitize BEFORE diffing: diffing first would let the 3 lines of context
	// carry the very secret the redaction is meant to remove.
	clean, redacted, filtered := SanitizeText(string(raw))

	// First sighting during the baseline scan: index it and say nothing.
	// A file we have never seen is not "a change", it is "the starting point".
	if !had && baseline {
		s.mu.Lock()
		if s.cached+int64(len(clean)) <= maxCachedBytes {
			s.cache[path] = &fileEntry{rawHash: rawHash, clean: clean, hasSecret: redacted, size: st.Size(), mtime: st.ModTime()}
			s.cached += int64(len(clean))
		} else {
			s.cache[path] = &fileEntry{rawHash: rawHash, hasSecret: redacted, size: st.Size(), mtime: st.ModTime()}
		}
		s.mu.Unlock()
		return
	}

	ev := NewEvent(model.ChangeKindConfig, "change.file", Object{Type: "file", Key: path}, st.ModTime())
	if ev.At.IsZero() {
		ev.At = time.Now()
	}
	ev.Actor = s.inferActor()
	ev.Object.Labels = labelsForFile(path, clean)

	d := &Diff{Changed: true, HashTo: rawHash}
	if had {
		d.HashFrom = prev.rawHash
		body, added, removed, truncated := MakeDiff(prev.clean, clean)
		d.Body, d.Added, d.Removed, d.Truncated = body, added, removed, truncated
		d.Fields = extractFields(prev.clean, clean, 12)
	} else {
		// Newly appeared file (not in the baseline index). Show its sanitized
		// content: this is exactly the case where the content matters most —
		// a webshell dropped into a config directory, a fresh sudoers entry.
		// Showing "(no baseline)" alone would hide the one thing worth reading.
		body, addedN, removedN, trunc := MakeDiff("", clean)
		d.Body = "(新增文件)\n" + body
		d.Added, d.Removed, d.Truncated = addedN, removedN, trunc
	}
	if filtered > 0 {
		ev.Object.Labels = append(ev.Object.Labels, "filtered_text")
		d.Body += "\n(已过滤 " + itoa(filtered) + " 处可疑文本)\n"
	}
	ev.Diff = d

	s.mu.Lock()
	if had {
		s.cached -= int64(len(prev.clean))
	}
	if s.cached+int64(len(clean)) <= maxCachedBytes {
		s.cache[path] = &fileEntry{rawHash: rawHash, clean: clean, hasSecret: redacted, size: st.Size(), mtime: st.ModTime()}
		s.cached += int64(len(clean))
	} else {
		s.cache[path] = &fileEntry{rawHash: rawHash, hasSecret: redacted, size: st.Size(), mtime: st.ModTime()}
	}
	s.mu.Unlock()

	// Even when the sanitized text is identical (only a secret value changed),
	// we still record it: "someone rotated the DB password" is exactly the kind
	// of fact that matters later. The body just shows no textual difference.
	if d.Added == 0 && d.Removed == 0 && had {
		d.Body = "(内容变化已脱敏，无可展示的文本差异)\n"
	}

	// Pre-change snapshot, filed under the final row id so rollback can find it.
	// Skipped for files that needed redaction: storing the plaintext of exactly
	// the secrets we refuse to record would defeat the sanitization design.
	id := s.persist(ev)
	if had && id != "" && s.snap != nil && s.cfg.Snapshot.Enabled && !prev.hasSecret {
		if err := s.snap.Save(id, path, []byte(prev.clean)); err != nil {
			log.Printf("change: snapshot %s: %v", path, err)
		}
	}
	_ = via
}

// inferActor consults the auth collector for sessions active right now. This is
// inference, never proof: without auditd we cannot know who ran the command.
func (s *fileSource) inferActor() Actor {
	if s.cfg.ActorResolver == nil {
		return Actor{Kind: "unknown", Confidence: model.ConfidenceUnknown}
	}
	if a, ok := s.cfg.ActorResolver(time.Now()); ok {
		a.Confidence = model.ConfidenceInferred
		return a
	}
	return Actor{Kind: "unknown", Confidence: model.ConfidenceUnknown}
}

func (s *fileSource) stop() {
	if s.watcher != nil {
		_ = s.watcher.Close()
	}
	s.mu.Lock()
	for _, t := range s.pending {
		t.Stop()
	}
	s.pending = map[string]*time.Timer{}
	s.mu.Unlock()
}

// labelsForFile derives correlation tags: path segments plus a few well-known
// directives (listen port, server_name) that make service-level matching work.
func labelsForFile(path, content string) []string {
	var labels []string
	parts := strings.Split(strings.Trim(filepath.Clean(path), "/"), "/")
	for _, p := range parts {
		if p == "" || p == "etc" || p == "conf" || p == "conf.d" || p == "sites-enabled" || p == "sites-available" {
			continue
		}
		p = strings.TrimSuffix(p, filepath.Ext(p))
		labels = append(labels, p)
		if len(labels) >= 6 {
			break
		}
	}
	for _, line := range strings.Split(content, "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "listen ") || strings.HasPrefix(l, "Listen ") {
			f := strings.Fields(l)
			if len(f) >= 2 {
				labels = append(labels, "port:"+strings.TrimSuffix(f[1], ";"))
			}
		}
		if strings.HasPrefix(l, "server_name ") {
			f := strings.Fields(strings.TrimSuffix(l, ";"))
			if len(f) >= 2 {
				labels = append(labels, "domain:"+f[1])
			}
		}
	}
	sort.Strings(labels)
	return labels
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// ensure io is referenced (io.Discard used by callers building readers)
var _ = io.Discard
