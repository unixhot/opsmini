package change

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// accountSource watches identity and privilege files.
//
// /etc/shadow and /etc/gshadow are reduced to "changed: true" — storing their
// content would copy password hashes into a web-facing application, which is
// exactly the kind of self-inflicted breach this design must avoid.
type accountSource struct {
	cfg   *Config
	emit  func(Event)
	mu    sync.Mutex
	known map[string]string // path -> sanitized content or hash marker
}

func newAccountSource(cfg *Config, emit func(Event)) *accountSource {
	return &accountSource{cfg: cfg, emit: emit, known: make(map[string]string)}
}

func (s *accountSource) Name() string { return "account" }

// accountFiles lists what is watched and how it is treated.
var accountFiles = []struct {
	path     string
	hashOnly bool // never store content
}{
	{"/etc/passwd", false},
	{"/etc/group", false},
	{"/etc/sudoers", false},
	{"/etc/shadow", true},
	{"/etc/gshadow", true},
}

func (s *accountSource) Run(ctx context.Context) {
	// Account changes are rare; a 30s poll is plenty and costs a few stat calls.
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	s.poll(true)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.poll(false)
		}
	}
}

func (s *accountSource) poll(baseline bool) {
	// /etc/sudoers.d/* is a directory, enumerate it separately
	paths := make([]string, 0, 16)
	for _, af := range accountFiles {
		if _, err := os.Stat(af.path); err == nil {
			paths = append(paths, af.path)
		}
	}
	if entries, err := os.ReadDir("/etc/sudoers.d"); err == nil {
		for _, e := range entries {
			if !e.IsDir() {
				paths = append(paths, filepath.Join("/etc/sudoers.d", e.Name()))
			}
		}
	}
	sort.Strings(paths)

	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, p := range paths {
		hashOnly := isHashOnly(p)
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		clean, _, _ := SanitizeText(string(data))
		if hashOnly {
			clean = "(内容不采集，仅记录变化)\n"
		}
		prev, existed := s.known[p]
		s.known[p] = clean
		if baseline || prev == clean {
			continue
		}
		// !existed is NOT skipped: a brand new /etc/sudoers.d/* file is one of the
		// highest-signal events there is (that is how a backdoor gets persistence),
		// so it must be recorded rather than treated as "no baseline, skip".
		ev := NewEvent(model.ChangeKindAccount, "change.account",
			Object{Type: "user", Key: p, Labels: accountLabels(p, clean)}, now)
		ev.Actor = Actor{Kind: "unknown", Confidence: model.ConfidenceUnknown}
		d := &Diff{Changed: true}
		if hashOnly {
			d.Body = "(敏感文件，仅记录发生变化)\n"
		} else if !existed {
			body, addedN, removedN, _ := MakeDiff("", clean)
			d.Body = "(新增文件)\n" + body
			d.Added, d.Removed = addedN, removedN
		} else {
			body, added, removed, truncated := MakeDiff(prev, clean)
			d.Body, d.Added, d.Removed, d.Truncated = body, added, removed, truncated
			d.Fields = extractFields(prev, clean, 12)
		}
		ev.Diff = d
		s.emit(ev)
	}
}

func isHashOnly(p string) bool {
	base := filepath.Base(p)
	return base == "shadow" || base == "gshadow"
}

// accountLabels highlights the security-relevant combinations: a new user that
// also lands in a privileged group is the classic persistence pattern.
func accountLabels(path, content string) []string {
	labels := []string{"account"}
	base := filepath.Base(path)
	labels = append(labels, base)
	if strings.Contains(path, "sudoers") {
		labels = append(labels, "sudo")
		if strings.Contains(content, "NOPASSWD") {
			labels = append(labels, "nopasswd")
		}
	}
	for _, g := range []string{"sudo", "wheel", "admin"} {
		if base == "group" && strings.Contains(content, g+":") {
			labels = append(labels, "group:"+g)
		}
	}
	return labels
}
