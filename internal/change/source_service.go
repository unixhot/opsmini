package change

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// serviceSource polls systemd unit states.
//
// Polling rather than subscribing to D-Bus: the bus would need a cgo binding or
// an extra client library, which conflicts with the pure-Go single-binary
// constraint. A 60s `systemctl list-units` costs ~20ms, which is nothing.
type serviceSource struct {
	cfg   *Config
	emit  func(Event)
	known map[string]string // unit -> "active/running"
}

func newServiceSource(cfg *Config, emit func(Event)) *serviceSource {
	return &serviceSource{cfg: cfg, emit: emit, known: make(map[string]string)}
}

func (s *serviceSource) Name() string { return "service" }

func (s *serviceSource) Run(ctx context.Context) {
	interval := s.cfg.Service.PollInterval
	if interval <= 0 {
		interval = time.Minute
	}
	t := time.NewTicker(interval)
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

func (s *serviceSource) poll(baseline bool) {
	out, err := runCmd(10*time.Second, "/usr/bin/systemctl",
		"list-units", "--type=service", "--all", "--no-legend", "--no-pager", "--plain")
	if err != nil && out == "" {
		return
	}
	cur := make(map[string]string, 256)
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(strings.TrimPrefix(line, "● "))
		if len(f) < 4 || !strings.HasSuffix(f[0], ".service") {
			continue
		}
		cur[f[0]] = f[2] + "/" + f[3] // ACTIVE/SUB
	}
	if len(cur) == 0 {
		return
	}
	if baseline || len(s.known) == 0 {
		s.known = cur
		return
	}
	now := time.Now()
	for unit, state := range cur {
		prev, existed := s.known[unit]
		if existed && prev == state {
			continue
		}
		// Ignore the noisy service set; without this every boot floods the log.
		if lowNoiseUnit(unit) {
			continue
		}
		ev := NewEvent(model.ChangeKindService, "change.service",
			Object{Type: "unit", Key: strings.TrimSuffix(unit, ".service"),
				Labels: []string{strings.TrimSuffix(unit, ".service")}}, now)
		ev.Actor = Actor{Kind: "system", How: "systemd", Confidence: model.ConfidenceConfirmed}
		if !existed {
			ev.Diff = &Diff{Changed: true, Body: "服务出现：" + state + "\n"}
		} else {
			ev.Diff = &Diff{Changed: true, Body: prev + " → " + state + "\n"}
		}
		s.emit(ev)
	}
	for unit, prev := range s.known {
		if _, ok := cur[unit]; ok || lowNoiseUnit(unit) {
			continue
		}
		ev := NewEvent(model.ChangeKindService, "change.service",
			Object{Type: "unit", Key: strings.TrimSuffix(unit, ".service"),
				Labels: []string{strings.TrimSuffix(unit, ".service")}}, now)
		ev.Actor = Actor{Kind: "system", How: "systemd", Confidence: model.ConfidenceConfirmed}
		ev.Diff = &Diff{Changed: true, Body: prev + " → 已移除\n"}
		s.emit(ev)
	}
	s.known = cur
}

// lowNoiseUnit filters units whose state flips constantly and carries no
// diagnostic value.
//
// user-runtime-dir@N is created and torn down on every login/logout, so keeping
// it would add one "change" per SSH session — real, but pure noise on a page
// whose job is to surface the change that actually matters.
func lowNoiseUnit(unit string) bool {
	for _, p := range []string{"systemd-", "dbus.", "getty@", "user@", "user-runtime-dir@", "session-", "polkit", "cron.service", "rsyslog.service"} {
		if strings.HasPrefix(unit, p) {
			return true
		}
	}
	return false
}

var _ = log.Printf
