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

// cronSource watches scheduled jobs. Cron is one of the most common persistence
// mechanisms after a compromise, so this is as much a security feed as an ops
// one.
type cronSource struct {
	cfg   *Config
	emit  func(Event)
	mu    sync.Mutex
	known map[string]string // "file|user" -> sanitized content
}

func newCronSource(cfg *Config, emit func(Event)) *cronSource {
	return &cronSource{cfg: cfg, emit: emit, known: make(map[string]string)}
}

func (s *cronSource) Name() string { return "cron" }

func (s *cronSource) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
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

// cronTargets returns (label, path) pairs, including per-user crontabs which live
// outside /etc and therefore are invisible to the file source.
func cronTargets() [][2]string {
	var out [][2]string
	for _, p := range []string{"/etc/crontab"} {
		if _, err := os.Stat(p); err == nil {
			out = append(out, [2]string{p, p})
		}
	}
	for _, dir := range []string{"/etc/cron.d", "/var/spool/cron", "/var/spool/cron/crontabs"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			full := filepath.Join(dir, e.Name())
			out = append(out, [2]string{full, full})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i][0] < out[j][0] })
	return out
}

func (s *cronSource) poll(baseline bool) {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, tgt := range cronTargets() {
		label, path := tgt[0], tgt[1]
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		clean, _, _ := SanitizeText(string(data))
		prev, existed := s.known[label]
		s.known[label] = clean
		if baseline || !existed || prev == clean {
			continue
		}
		ev := NewEvent(model.ChangeKindCron, "change.cron",
			Object{Type: "job", Key: label, Labels: cronLabels(clean)}, now)
		ev.Actor = Actor{Kind: "unknown", Confidence: model.ConfidenceUnknown}
		body, added, removed, truncated := MakeDiff(prev, clean)
		ev.Diff = &Diff{Changed: true, Body: body, Added: added, Removed: removed, Truncated: truncated}
		// Surface the part that matters: the new schedule line itself.
		if added > 0 {
			ev.Object.Labels = append(ev.Object.Labels, "job_added")
			if line := firstAddedLine(prev, clean); line != "" {
				ev.Diff.Body = "新增任务：" + line + "\n" + body
			}
		}
		s.emit(ev)
	}
	// systemd timers, if any
	if out, err := runCmd(10*time.Second, "/usr/bin/systemctl", "list-timers", "--all", "--no-legend", "--no-pager"); err == nil {
		clean := timerIdentity(out)
		prev, existed := s.known["systemd-timers"]
		s.known["systemd-timers"] = clean
		if !baseline && existed && prev != clean {
			ev := NewEvent(model.ChangeKindCron, "change.cron",
				Object{Type: "job", Key: "systemd-timers", Labels: []string{"timer"}}, now)
			body, added, removed, truncated := MakeDiff(prev, clean)
			ev.Diff = &Diff{Changed: true, Body: body, Added: added, Removed: removed, Truncated: truncated}
			s.emit(ev)
		}
	}
}

// timerIdentity reduces `systemctl list-timers` output to just the timer and the
// unit it activates.
//
// The full table contains NEXT / LEFT / LAST / PASSED, which change on every
// single call ("1h 20min left" becomes "1h 19min left"). Comparing the raw output
// therefore reports a change every minute forever — a pure noise generator that
// buries real changes. Only identity matters here.
func timerIdentity(out string) string {
	var b strings.Builder
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 {
			continue
		}
		// The unit name is the first token ending in .timer; the activated unit
		// follows it. Everything before it is timing information.
		for i, tok := range f {
			if strings.HasSuffix(tok, ".timer") {
				b.WriteString(strings.Join(f[i:], " "))
				b.WriteString("\n")
				break
			}
		}
	}
	return b.String()
}

// cronLabels flags the shapes that usually mean a backdoor rather than a job.
func cronLabels(content string) []string {
	labels := []string{"cron"}
	lower := strings.ToLower(content)
	for _, bad := range []string{"curl ", "wget ", "bash -i", "/dev/tcp/", "nc -e", "base64 -d", "python -c"} {
		if strings.Contains(lower, bad) {
			labels = append(labels, "suspicious_command")
			break
		}
	}
	if strings.Contains(content, "* * * * *") {
		labels = append(labels, "every_minute")
	}
	return labels
}

// firstAddedLine returns the first non-empty line present in after but not in
// before, so the summary can say "新增任务：* * * * * curl ...".
func firstAddedLine(before, after string) string {
	exist := make(map[string]struct{}, 32)
	for _, l := range strings.Split(before, "\n") {
		exist[l] = struct{}{}
	}
	for _, l := range strings.Split(after, "\n") {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if _, ok := exist[l]; !ok {
			return t
		}
	}
	return ""
}
