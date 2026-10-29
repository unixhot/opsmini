package change

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// Package sources read package manager logs incrementally. Parsing a 200MB
// dpkg.log on every start would be silly, so a byte offset is persisted and only
// new lines are examined.
type pkgSource struct {
	cfg   *Config
	store *Store
	emit  func(Event)
}

func newPackageSource(cfg *Config, store *Store, emit func(Event)) *pkgSource {
	return &pkgSource{cfg: cfg, store: store, emit: emit}
}

func (s *pkgSource) Name() string { return "package" }

// logCandidates is ordered by preference; the first readable one wins.
var logCandidates = []struct {
	path string
	kind string // dpkg | dnf
}{
	{"/var/log/dpkg.log", "dpkg"},
	{"/var/log/dnf.log", "dnf"},
	{"/var/log/yum.log", "dnf"},
}

func (s *pkgSource) Run(ctx context.Context) {
	// Two different cadences on purpose:
	//   - log tailing every 30s: an `apt install` should show up in the timeline
	//     within seconds, that is the whole point of the feature.
	//   - full package snapshot every SnapshotInterval (30m): it costs a few
	//     hundred ms and only exists as a safety net for rotated/missing logs.
	// Tying both to one 30 minute ticker meant an install was invisible for half
	// an hour, which makes the feature useless for its main use case.
	logTick := time.NewTicker(30 * time.Second)
	defer logTick.Stop()
	snapInterval := s.cfg.Package.SnapshotInterval
	if snapInterval <= 0 {
		snapInterval = 30 * time.Minute
	}
	snapTick := time.NewTicker(snapInterval)
	defer snapTick.Stop()

	s.pollLogs()
	for {
		select {
		case <-ctx.Done():
			return
		case <-logTick.C:
			s.pollLogs()
		case <-snapTick.C:
			s.snapshotPackages()
		}
	}
}

func (s *pkgSource) pollLogs() {
	for _, cand := range logCandidates {
		st, err := os.Stat(cand.path)
		if err != nil {
			continue
		}
		ino := inodeOf(st)
		prev, had := s.store.Offset(cand.path)
		offset := int64(0)
		if had && prev.Inode == ino && prev.Offset <= st.Size() {
			offset = prev.Offset // continuation
		}
		// rotation or first run: start from the end minus a small tail so we do
		// not replay years of history on a fresh install
		if !had || prev.Inode != ino {
			if st.Size() > 64*1024 {
				offset = st.Size() - 64*1024
			}
		}
		f, err := os.Open(cand.path)
		if err != nil {
			continue
		}
		if _, err := f.Seek(offset, 0); err != nil {
			f.Close()
			continue
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		if offset > 0 {
			// skip a partial line
			sc.Scan()
		}
		for sc.Scan() {
			line := sc.Text()
			if cand.kind == "dpkg" {
				s.parseDpkgLine(line)
			} else {
				s.parseDnfLine(line)
			}
		}
		newOffset, _ := f.Seek(0, 1)
		f.Close()
		s.store.SaveOffset(cand.path, ino, newOffset)
		return // only the first available log
	}
}

// parseDpkgLine handles:
//
//	2026-10-28 04:12:33 upgrade openssl:amd64 3.0.11-1 3.0.13-1
//	2026-10-28 04:12:33 install foo:amd64 <none> 1.0-1
//	2026-10-28 04:12:33 remove bar:amd64 1.0-1 <none>
func (s *pkgSource) parseDpkgLine(line string) {
	f := strings.Fields(line)
	if len(f) < 4 {
		return
	}
	action := f[2]
	if action != "install" && action != "upgrade" && action != "remove" && action != "purge" {
		return
	}
	pkg := f[3]
	if i := strings.Index(pkg, ":"); i > 0 {
		pkg = pkg[:i]
	}
	from, to := "", ""
	if len(f) >= 6 {
		from, to = f[4], f[5]
	}
	at := parseDpkgTime(f[0] + " " + f[1])
	ev := NewEvent(model.ChangeKindPackage, "change.package",
		Object{Type: "package", Key: pkg, Labels: []string{pkg, action}}, at)
	ev.Actor = Actor{Kind: "package_manager", How: "dpkg", Confidence: model.ConfidenceConfirmed}
	desc := fmt.Sprintf("%s %s", action, pkg)
	if from != "" && to != "" && from != "<none>" {
		desc = fmt.Sprintf("%s %s %s → %s", action, pkg, from, to)
	}
	ev.Diff = &Diff{Changed: true, Body: desc + "\n"}
	s.emit(ev)
}

// parseDnfLine handles lines like:
//
//	2026-10-28T04:12:33+0800 Upgrade openssl-3.0.11-1.el9.x86_64 ...
func (s *pkgSource) parseDnfLine(line string) {
	if !strings.Contains(line, "Upgrade") && !strings.Contains(line, "Install") &&
		!strings.Contains(line, "Erase") && !strings.Contains(line, "Removed") {
		return
	}
	f := strings.Fields(line)
	if len(f) < 3 {
		return
	}
	var action string
	for _, w := range f[:4] {
		switch w {
		case "Upgrade", "Upgraded":
			action = "upgrade"
		case "Install", "Installed":
			action = "install"
		case "Erase", "Erased", "Removed":
			action = "remove"
		}
		if action != "" {
			break
		}
	}
	if action == "" {
		return
	}
	pkg := f[2]
	if i := strings.Index(pkg, "-"); i > 0 {
		pkg = pkg[:i]
	}
	at := parseDnfTime(f[0])
	ev := NewEvent(model.ChangeKindPackage, "change.package",
		Object{Type: "package", Key: pkg, Labels: []string{pkg, action}}, at)
	ev.Actor = Actor{Kind: "package_manager", How: "dnf", Confidence: model.ConfidenceConfirmed}
	ev.Diff = &Diff{Changed: true, Body: action + " " + pkg + "\n"}
	s.emit(ev)
}

// snapshotPackages compares the installed package set against the last known
// one. Only differences are emitted, so a quiet machine costs nothing.
func (s *pkgSource) snapshotPackages() {
	list := installedPackages()
	if len(list) == 0 {
		return
	}
	cur := strings.Join(list, "\n")
	sum := shortHash(cur)

	var prevPkgSnap struct{ Value string }
	s.store.db.Raw("SELECT value FROM settings WHERE key = 'change_pkg_snapshot' LIMIT 1").Scan(&prevPkgSnap)
	if prevPkgSnap.Value == "" {
		s.store.db.Exec("INSERT INTO settings(key, value) VALUES('change_pkg_snapshot', ?)", sum)
		return
	}
	if prevPkgSnap.Value == sum {
		return
	}
	// The set changed but the log parse already covered it in the normal case;
	// this branch mainly catches machines whose logs are unreadable.
	s.store.db.Exec("UPDATE settings SET value = ? WHERE key = 'change_pkg_snapshot'", sum)
}

func installedPackages() []string {
	type cmd struct {
		bin  string
		args []string
	}
	cands := []cmd{
		{"/usr/bin/dpkg-query", []string{"-W", "-f=${Package} ${Version}\n"}},
		{"/usr/bin/rpm", []string{"-qa"}},
	}
	for _, c := range cands {
		if _, err := os.Stat(c.bin); err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		out, err := exec.CommandContext(ctx, c.bin, c.args...).Output()
		cancel()
		if err != nil {
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		return lines
	}
	return nil
}

func parseDpkgTime(s string) time.Time {
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t
	}
	return time.Now()
}

func parseDnfTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02T15:04:05-0700", "2006-01-02T15:04:05Z07:00"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Now()
}
