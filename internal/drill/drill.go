// Package drill injects real faults on the host so operators can watch the
// agent notice and handle them, then restores the machine automatically.
//
// The point is not to have six buttons; it is to answer one question before
// anyone dares to switch a policy to auto: "when this breaks, does the system
// actually notice, and does the remedy work?" Making the fault requires no
// cleverness — leaving the machine in that state does. So every drill carries a
// deadline and the manager reconciles leftovers on startup.
package drill

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Kind identifies a fault.
type Kind string

const (
	KindCPUHog     Kind = "cpu_hog"
	KindLogFlood   Kind = "log_flood"
	KindWebshell   Kind = "webshell"
	KindDiskFill   Kind = "disk_fill"
	KindDockerDown Kind = "docker_down"
	KindCronFail   Kind = "cron_fail"
)

// Risk decides whether the caller must pass an explicit confirmation.
const (
	// RiskSafe faults cost CPU/disk/log space but cannot break other services,
	// and restored cleanly they leave no trace.
	RiskSafe = "safe"
	// RiskGuard faults touch shared resources (the docker daemon, a nearly full
	// filesystem). They are legitimate drills, but not something to trigger by
	// a stray click.
	RiskGuard = "guard"
)

// Spec is the static description of a drill, used by the API and the UI.
type Spec struct {
	Kind Kind `json:"kind"`
	// Risk is "safe" or "guard"; guard requires {"confirm": true}.
	Risk string `json:"risk"`
	// Detector names the collector that is expected to notice this fault. A drill
	// whose detector cannot observe it is worth knowing about: it marks a blind
	// spot rather than a working capability.
	Detector string `json:"detector"`
	// TTLSeconds is how long the fault is left in place before auto-restore.
	TTLSeconds int `json:"ttl_seconds"`
}

// specs is ordered: the UI lists them in this order.
var specs = []Spec{
	{Kind: KindCPUHog, Risk: RiskSafe, Detector: "metric", TTLSeconds: 600},
	{Kind: KindLogFlood, Risk: RiskSafe, Detector: "logwatch", TTLSeconds: 600},
	{Kind: KindWebshell, Risk: RiskSafe, Detector: "fim", TTLSeconds: 900},
	{Kind: KindCronFail, Risk: RiskSafe, Detector: "alertcheck", TTLSeconds: 600},
	{Kind: KindDiskFill, Risk: RiskGuard, Detector: "metric", TTLSeconds: 300},
	{Kind: KindDockerDown, Risk: RiskGuard, Detector: "alertcheck", TTLSeconds: 180},
}

// Specs returns a copy so callers cannot mutate the table.
func Specs() []Spec {
	out := make([]Spec, len(specs))
	copy(out, specs)
	return out
}

// SpecFor looks a drill up by kind.
func SpecFor(k Kind) (Spec, bool) {
	for _, s := range specs {
		if s.Kind == k {
			return s, true
		}
	}
	return Spec{}, false
}

// Status is the live view of one drill.
type Status struct {
	Kind      Kind       `json:"kind"`
	Active    bool       `json:"active"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	StopAt    *time.Time `json:"stop_at,omitempty"`
	// Note carries anything the operator needs to interpret the result, e.g.
	// "the target path is not under FIM monitoring, so no event will appear".
	Note string `json:"note,omitempty"`
	// Error is the last failure of inject/restore, if any.
	Error string `json:"error,omitempty"`
}

// Run is the bookkeeping for one active fault.
type Run struct {
	Kind      Kind      `json:"kind"`
	StartedAt time.Time `json:"started_at"`
	Deadline  time.Time `json:"deadline"`

	// Files and PIDs let a restarted process clean up what its predecessor left
	// behind. Anything created by an inject must be listed here.
	Files []string `json:"files,omitempty"`
	PIDs  []int    `json:"pids,omitempty"`
	Note  string   `json:"note,omitempty"`
	// Meta holds op-specific handles (a transient unit name, a cron job id) so
	// restore can be replayed from a different process.
	Meta map[string]string `json:"meta,omitempty"`

	// stop cancels whatever background loop the op started.
	stop context.CancelFunc `json:"-"`
}

// Op is one injectable fault. Tests supply fakes; production uses the real ones
// in ops.go.
type Op interface {
	Kind() Kind
	// Inject creates the fault. It should fill r.Files / r.PIDs so restore works
	// even from another process, and may set r.Note.
	Inject(ctx context.Context, r *Run) error
	// Restore removes the fault. It must be idempotent: a machine that was
	// rebooted mid-drill still ends up clean.
	Restore(ctx context.Context, r *Run) error
}

// Manager owns the drill state.
type Manager struct {
	mu     sync.Mutex
	ops    map[Kind]Op
	active map[Kind]*Run
	// errs remembers the last error per kind so the UI can show why a start or
	// restore failed instead of silently doing nothing.
	errs map[Kind]string

	statePath string
	now       func() time.Time
	// ttl overrides the per-spec deadline. Tests set it to a few milliseconds;
	// production leaves it zero and uses the spec.
	ttl  time.Duration
	logf func(string, ...any)
	// stopAll lets Close() cancel background writers.
	closed bool
}

// New builds a manager. stateDir is where the state file lives (inside the
// panel data directory); pass "" to keep state in memory only.
func New(stateDir string, ops ...Op) *Manager {
	m := &Manager{
		ops:    map[Kind]Op{},
		active: map[Kind]*Run{},
		errs:   map[Kind]string{},
		now:    time.Now,
		logf:   func(string, ...any) {},
	}
	for _, o := range ops {
		m.ops[o.Kind()] = o
	}
	if stateDir != "" {
		m.statePath = filepath.Join(stateDir, "drill-state.json")
	}
	return m
}

// SetLogger wires a logger (the panel passes its standard logger).
func (m *Manager) SetLogger(f func(string, ...any)) {
	if f != nil {
		m.logf = f
	}
}

// Reconcile cleans up faults left by a previous process. Called at startup,
// because a drill that survives a crash is just an outage.
func (m *Manager) Reconcile(ctx context.Context) {
	if m.statePath == "" {
		return
	}
	b, err := os.ReadFile(m.statePath)
	if err != nil {
		return
	}
	var runs []*Run
	if err := json.Unmarshal(b, &runs); err != nil {
		m.logf("drill: 状态文件损坏，跳过清理: %v", err)
		return
	}
	for _, r := range runs {
		op := m.ops[r.Kind]
		if op == nil {
			// The op disappeared (build without that capability): still remove the
			// artefacts so the machine does not carry a mystery file forever.
			removeFiles(r.Files)
			continue
		}
		if err := op.Restore(ctx, r); err != nil {
			// 清理失败就把它留在 active 里：故障还在机器上，界面必须显示出来，
			// 人也还能再点一次「恢复」。偷偷从状态文件里抹掉才是真的危险。
			m.logf("drill: 启动清理 %s 失败: %v", r.Kind, err)
			m.mu.Lock()
			m.active[r.Kind] = r
			m.errs[r.Kind] = "启动清理失败: " + err.Error()
			m.mu.Unlock()
			continue
		}
		m.logf("drill: 启动时清理了上次残留的 %s", r.Kind)
	}
	m.save()
}

// Start injects a fault. confirm must be true for RiskGuard drills.
func (m *Manager) Start(ctx context.Context, k Kind, confirm bool) (Status, error) {
	spec, ok := SpecFor(k)
	if !ok {
		return Status{}, fmt.Errorf("未知的演练类型: %s", k)
	}
	if spec.Risk == RiskGuard && !confirm {
		return Status{}, errors.New("该演练会影响其它服务，需要显式确认")
	}
	op := m.ops[k]
	if op == nil {
		return Status{}, errors.New("当前环境不支持该演练")
	}

	m.mu.Lock()
	if _, running := m.active[k]; running {
		m.mu.Unlock()
		return Status{}, errors.New("该演练正在进行中")
	}
	// Two faults at once makes the observation useless: you cannot tell which
	// one caused the signal. One at a time, on purpose.
	if len(m.active) > 0 {
		var other Kind
		for ok := range m.active {
			other = ok
		}
		m.mu.Unlock()
		return Status{}, fmt.Errorf("已有演练 %s 在进行，请先恢复", other)
	}
	ttl := time.Duration(spec.TTLSeconds) * time.Second
	if m.ttl > 0 {
		ttl = m.ttl
	}
	m.active[k] = &Run{Kind: k, StartedAt: m.now(), Deadline: m.now().Add(ttl)}
	m.mu.Unlock()

	runCtx, cancel := context.WithCancel(context.Background())
	m.mu.Lock()
	r := m.active[k]
	r.stop = cancel
	m.mu.Unlock()

	if err := op.Inject(runCtx, r); err != nil {
		cancel()
		m.mu.Lock()
		delete(m.active, k)
		m.errs[k] = err.Error()
		m.mu.Unlock()
		// Best effort: a half-injected fault must not stay behind.
		_ = op.Restore(context.Background(), r)
		m.save()
		return Status{}, err
	}
	m.save()
	m.logf("drill: 已注入 %s，%s 后自动恢复", k, ttl)

	// Auto-restore. Deliberately not cancellable by the request context: the
	// caller hanging up must not leave the fault running.
	time.AfterFunc(ttl, func() {
		if _, err := m.Stop(context.Background(), k); err != nil {
			m.logf("drill: %s 自动恢复失败: %v", k, err)
		}
	})
	return m.status(k), nil
}

// Stop restores the machine. Idempotent: stopping a non-running drill is fine.
func (m *Manager) Stop(ctx context.Context, k Kind) (Status, error) {
	m.mu.Lock()
	r := m.active[k]
	m.mu.Unlock()
	if r == nil {
		return m.status(k), nil
	}
	op := m.ops[k]
	if op == nil {
		return m.status(k), errors.New("当前环境不支持该演练")
	}
	if err := op.Restore(ctx, r); err != nil {
		m.mu.Lock()
		m.errs[k] = "恢复失败: " + err.Error()
		m.mu.Unlock()
		m.save()
		return m.status(k), err
	}
	if r.stop != nil {
		r.stop()
	}
	m.mu.Lock()
	delete(m.active, k)
	delete(m.errs, k)
	m.mu.Unlock()
	m.save()
	m.logf("drill: %s 已恢复", k)
	return m.status(k), nil
}

// List returns every drill with its current state.
func (m *Manager) List() []Status {
	out := make([]Status, 0, len(specs))
	for _, s := range specs {
		out = append(out, m.status(s.Kind))
	}
	sort.SliceStable(out, func(i, j int) bool {
		for _, s := range specs {
			if s.Kind == out[i].Kind {
				return true
			}
			if s.Kind == out[j].Kind {
				return false
			}
		}
		return false
	})
	return out
}

func (m *Manager) status(k Kind) Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := Status{Kind: k, Error: m.errs[k]}
	if r, ok := m.active[k]; ok {
		started, deadline := r.StartedAt, r.Deadline
		st.Active = true
		st.StartedAt = &started
		st.StopAt = &deadline
		st.Note = r.Note
	}
	return st
}

// Close restores everything still running (panel shutdown).
func (m *Manager) Close(ctx context.Context) {
	m.mu.Lock()
	kinds := make([]Kind, 0, len(m.active))
	for k := range m.active {
		kinds = append(kinds, k)
	}
	m.closed = true
	m.mu.Unlock()
	for _, k := range kinds {
		_, _ = m.Stop(ctx, k)
	}
}

func (m *Manager) save() {
	if m.statePath == "" {
		return
	}
	m.mu.Lock()
	runs := make([]*Run, 0, len(m.active))
	for _, r := range m.active {
		c := *r
		c.stop = nil
		runs = append(runs, &c)
	}
	m.mu.Unlock()
	if len(runs) == 0 {
		_ = os.Remove(m.statePath)
		return
	}
	b, err := json.MarshalIndent(runs, "", " ")
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.statePath), 0o755); err != nil {
		return
	}
	_ = os.WriteFile(m.statePath, b, 0o600)
}

func removeFiles(paths []string) {
	for _, p := range paths {
		if p != "" {
			_ = os.RemoveAll(p)
		}
	}
}
