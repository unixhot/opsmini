package sentinel

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

var (
	reAccepted = regexp.MustCompile(`Accepted (\S+) for (\S+) from ([\w\.:]+) port (\d+)`)
	reFailed   = regexp.MustCompile(`Failed (\S+) for (?:invalid user )?(\S*) from ([\w\.:]+) port (\d+)`)
	reInvalid  = regexp.MustCompile(`Invalid user (\S+) from ([\w\.:]+)`)
)

// authWatcher tails the authentication log and turns logins into signals.
//
// The features it extracts are deliberately simple and explainable: off-hours,
// never-seen-before source address, failures right before a success, and an
// authorized_keys change in the last 30 minutes. Those four cover the common
// "someone got in" stories without needing a kernel probe.
type authWatcher struct {
	cfg   Config
	db    *gorm.DB
	mu    sync.Mutex
	path  string
	off   int64
	inode uint64
	fails map[string][]time.Time
}

func newAuthWatcher(cfg Config, db *gorm.DB) *authWatcher {
	return &authWatcher{cfg: cfg, db: db, fails: map[string][]time.Time{}}
}

func (w *authWatcher) Name() string { return "authwatch" }

func (w *authWatcher) Run(ctx context.Context, out func(Signal)) {
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.scan(out)
		}
	}
}

func (w *authWatcher) logPath() string {
	for _, p := range []string{"/var/log/auth.log", "/var/log/secure"} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func (w *authWatcher) scan(out func(Signal)) {
	path := w.logPath()
	if path == "" {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	st, err := os.Stat(path)
	if err != nil {
		return
	}
	inode := inodeOf(st)
	if w.path != path || w.inode != inode || st.Size() < w.off {
		w.path, w.inode, w.off = path, inode, st.Size()
		// Start from the tail on first sight: history is not our business.
		return
	}
	if st.Size() == w.off {
		return
	}

	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Seek(w.off, io.SeekStart); err != nil {
		return
	}
	buf := make([]byte, 1<<20)
	n, _ := f.Read(buf)
	w.off += int64(n)
	if n == 0 {
		return
	}

	for _, line := range strings.Split(string(buf[:n]), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		w.handleLine(line, out)
	}
}

func (w *authWatcher) handleLine(line string, out func(Signal)) {
	if m := reFailed.FindStringSubmatch(line); m != nil {
		ip := m[3]
		w.fails[ip] = append(w.fails[ip], time.Now())
		w.prune(ip)
		if len(w.fails[ip]) >= w.cfg.AuthFailBurst {
			out(newSignal(model.SignalKindAuth, "authwatch/sshd", ip, "auth.anomaly",
				"auth.failed_burst", model.SeverityMedium,
				"登录失败激增：来自 "+ip+" 的失败已达 "+itoa(int64(len(w.fails[ip])))+" 次").
				withTags("mitre:T1110", "scenario:ssh_bruteforce").
				withEvidence(map[string]any{"ip": ip, "fail_count": len(w.fails[ip]), "last_line": truncate(line, 300)}))
			// Reset so the next burst is counted separately.
			w.fails[ip] = nil
		}
		return
	}
	if m := reInvalid.FindStringSubmatch(line); m != nil {
		ip := m[2]
		w.fails[ip] = append(w.fails[ip], time.Now())
		w.prune(ip)
		return
	}
	m := reAccepted.FindStringSubmatch(line)
	if m == nil {
		return
	}
	method, user, ip := m[1], m[2], m[3]
	now := time.Now()
	offHours := w.isOffHours(now)
	newIP := !w.ipSeen(ip)
	recentFails := 0
	for _, t := range w.fails[ip] {
		if now.Sub(t) <= 10*time.Minute {
			recentFails++
		}
	}
	keyChanged := authorizedKeysChanged(30 * time.Minute)

	sev := model.SeverityInfo
	reason := "常规登录"
	switch {
	case recentFails >= 5:
		sev, reason = model.SeverityCritical, "爆破后成功登录"
	case keyChanged:
		sev, reason = model.SeverityCritical, "authorized_keys 刚被修改随后登录"
	case offHours && newIP:
		sev, reason = model.SeverityHigh, "非工作时段 + 首次出现的来源地址"
	case newIP:
		sev, reason = model.SeverityMedium, "首次出现的来源地址"
	case offHours:
		sev, reason = model.SeverityMedium, "非工作时段登录"
	}
	if ev := newIP; ev {
		w.markIPSeen(ip)
	}
	out(newSignal(model.SignalKindAuth, "authwatch/sshd", ip, "auth.anomaly",
		"auth.accepted", sev,
		"SSH 登录："+user+" 来自 "+ip+"（"+reason+"）").
		withTags("mitre:T1078", "scenario:ssh_login").
		withEvidence(map[string]any{
			"user": user, "ip": ip, "method": method,
			"off_hours": offHours, "new_ip": newIP,
			"recent_failures": recentFails, "authorized_keys_changed": keyChanged,
			"reason": reason, "line": truncate(line, 300),
		}))
}

func (w *authWatcher) prune(ip string) {
	cut := time.Now().Add(-1 * time.Hour)
	list := w.fails[ip]
	keep := list[:0]
	for _, t := range list {
		if t.After(cut) {
			keep = append(keep, t)
		}
	}
	w.fails[ip] = keep
}

func (w *authWatcher) isOffHours(t time.Time) bool {
	h := t.Hour()
	start, end := w.cfg.OffHoursStart, w.cfg.OffHoursEnd
	if start == end {
		return false
	}
	if start < end {
		return h >= start && h < end
	}
	// Wraps midnight (for example 22 -> 6).
	return h >= start || h < end
}

func (w *authWatcher) ipSeen(ip string) bool {
	if w.db == nil {
		return false
	}
	var n int64
	w.db.Model(&model.Baseline{}).Where("key = ?", "auth_ip:"+ip).Count(&n)
	return n > 0
}

func (w *authWatcher) markIPSeen(ip string) {
	if w.db == nil {
		return
	}
	w.db.Save(&model.Baseline{
		Key:       "auth_ip:" + ip,
		Kind:      "auth_ip",
		Value:     float64(time.Now().Unix()),
		Samples:   1,
		UpdatedAt: time.Now(),
	})
}

// authorizedKeysChanged reports whether any authorized_keys file changed recently.
func authorizedKeysChanged(window time.Duration) bool {
	cut := time.Now().Add(-window)
	check := func(p string) bool {
		st, err := os.Stat(p)
		return err == nil && st.ModTime().After(cut)
	}
	if check("/root/.ssh/authorized_keys") {
		return true
	}
	entries, err := os.ReadDir("/home")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if check(filepath.Join("/home", e.Name(), ".ssh", "authorized_keys")) {
			return true
		}
	}
	return false
}
