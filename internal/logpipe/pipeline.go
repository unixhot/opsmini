package logpipe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// Manager owns the log pipeline: it scans tasks.d, runs one reader goroutine per
// task, and publishes batches through a single output.
//
// Nothing starts until a task file exists. On a standalone host that means the
// module costs zero goroutines and zero bytes — which is the intended behaviour:
// log collection is a role OpsMini plays when managed, not a feature every
// install pays for.
type Manager struct {
	cfg     MainConfig
	reg     *Registry
	db      *gorm.DB
	version string

	mu       sync.RWMutex
	tasks    map[string]*taskRuntime
	warnings []string
	loadedAt time.Time

	out    Output
	outErr string

	sendQ chan batch
	stop  chan struct{}
}

type taskRuntime struct {
	task  Task
	hash  string
	procs []Processor
	state string
}

// TaskStatus is the per-task view shown in the panel and reported to OpsAnt.
type TaskStatus struct {
	TaskID        string    `json:"task_id"`
	File          string    `json:"file"`
	Paths         []string  `json:"paths"`
	Output        string    `json:"output"`
	State         string    `json:"state"`
	FilesOpen     int       `json:"files_open"`
	BytesRead     int64     `json:"bytes_read"`
	EventsSent    int64     `json:"events_sent"`
	EventsDropped int64     `json:"events_dropped"`
	LastError     string    `json:"last_error"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// PipelineStatus is the whole-pipeline view.
type PipelineStatus struct {
	Enabled   bool         `json:"enabled"`
	TasksDir  string       `json:"tasks_dir"`
	Tasks     []TaskStatus `json:"tasks"`
	Output    string       `json:"output"`
	OutputErr string       `json:"output_error"`
	Warnings  []string     `json:"warnings"`
	LoadedAt  time.Time    `json:"loaded_at"`
	Stats     OutputStats  `json:"output_stats"`
}

// stats holds the per-task counters that survive a task restart.
type stats struct {
	filesOpen     int64
	bytesRead     int64
	eventsSent    int64
	eventsDropped int64
}

var taskStats sync.Map // taskID -> *stats

func statsOf(id string) *stats {
	if v, ok := taskStats.Load(id); ok {
		return v.(*stats)
	}
	s := &stats{}
	taskStats.Store(id, s)
	return s
}

// NewManager builds the pipeline. Output errors are kept (not fatal): a broken
// ES host must not stop the agent from starting.
func NewManager(cfg MainConfig, db *gorm.DB, version string) *Manager {
	m := &Manager{
		cfg:     cfg,
		reg:     NewRegistry(db),
		db:      db,
		version: version,
		tasks:   map[string]*taskRuntime{},
		sendQ:   make(chan batch, 256),
		stop:    make(chan struct{}),
	}
	if out, err := newOutput(cfg.Output); err != nil {
		if cfg.Output.Type != "" {
			m.outErr = err.Error()
			log.Printf("logpipe: output unavailable: %v", err)
		}
	} else {
		m.out = out
	}
	return m
}

// Start launches the scanner and the sender. It returns immediately.
func (m *Manager) Start(ctx context.Context) {
	if !m.cfg.Enabled {
		return
	}
	if err := os.MkdirAll(m.cfg.TasksDir, 0o755); err != nil {
		log.Printf("logpipe: create tasks dir: %v", err)
	}
	go m.sender(ctx)
	go m.loop(ctx)
}

// loop reloads tasks.d and reconciles running readers.
func (m *Manager) loop(ctx context.Context) {
	period := m.cfg.ReloadPeriod
	if period <= 0 {
		period = 10 * time.Second
	}
	m.reload()
	t := time.NewTicker(period)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			m.stopAll()
			return
		case <-m.stop:
			m.stopAll()
			return
		case <-t.C:
			m.reload()
		}
	}
}

// reload compares tasks.d against the running set. A task whose file content did
// not change keeps running untouched — restarting it would re-open files and
// lose the multiline buffer for no reason.
func (m *Manager) reload() {
	tasks, warnings := LoadTasks(m.cfg.TasksDir, m.cfg.Strict)
	next := make(map[string]*taskRuntime, len(tasks))
	for i := range tasks {
		t := tasks[i]
		h := taskHash(t)
		procs, ws := buildProcessors(t.Input.Processors)
		warnings = append(warnings, ws...)
		next[t.ID] = &taskRuntime{task: t, hash: h, procs: procs, state: "running"}
	}

	m.mu.Lock()
	old := m.tasks
	m.tasks = next
	m.warnings = warnings
	m.loadedAt = time.Now()
	m.mu.Unlock()

	// Stop readers of removed or changed tasks.
	for id := range old {
		if _, ok := next[id]; !ok {
			log.Printf("logpipe: 任务 %s 已移除", id)
		}
	}

	for id, rt := range next {
		if prev, ok := old[id]; ok && prev.hash == rt.hash {
			// Unchanged: keep the existing goroutine by swapping the runtime in.
			continue
		}
		go m.runTask(id)
		if _, ok := old[id]; !ok && len(next) > 0 {
			log.Printf("logpipe: 任务 %s 已加载（%s）", id, strings.Join(rt.task.Input.Paths, ","))
		}
	}
}

func taskHash(t Task) string {
	parts := []string{t.ID, strings.Join(t.Input.Paths, ","), fmt.Sprint(len(t.Input.Processors))}
	if data, err := os.ReadFile(t.File); err == nil {
		sum := sha256.Sum256(data)
		parts = append(parts, hex.EncodeToString(sum[:8]))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:8])
}

// runTask keeps reading the task's files until the task disappears.
func (m *Manager) runTask(id string) {
	st := statsOf(id)
	for {
		m.mu.RLock()
		rt, ok := m.tasks[id]
		m.mu.RUnlock()
		if !ok {
			return
		}
		select {
		case <-m.stop:
			return
		default:
		}

		files := expandPaths(rt.task.Input.Paths, rt.task.Input.ExcludeFiles)
		atomic.StoreInt64(&st.filesOpen, int64(len(files)))
		for _, path := range files {
			h := &harvester{task: rt.task, path: path, reg: m.reg, agent: m.cfg.AgentType, version: m.version}
			batches, err := h.run(context.Background())
			if err != nil {
				m.setErr(id, err.Error())
				continue
			}
			for _, b := range batches {
				b := b
				select {
				case m.sendQ <- b:
				case <-m.stop:
					return
				}
			}
		}
		// Rescan quickly for an active file; close_inactive is honoured by
		// skipping files that have not changed, not by sleeping less.
		period := rt.task.Input.ScanFrequency
		if period <= 0 {
			period = time.Second
		}
		select {
		case <-m.stop:
			return
		case <-time.After(period):
		}
	}
}

// sender publishes batches and advances the registry only on success.
func (m *Manager) sender(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stop:
			return
		case b := <-m.sendQ:
			if m.out == nil {
				// No output yet: drop would lose data, blocking would fill the
				// queue. Hold and retry once the output is fixed.
				time.Sleep(2 * time.Second)
				select {
				case m.sendQ <- b:
				default:
				}
				continue
			}
			events := b.events
			// Run the processor chain here (not in the reader) so a slow
			// processor cannot slow down file reading.
			events = m.applyProcessors(b.task.ID, events)
			if len(events) == 0 {
				m.reg.Advance(b.task.ID, b.path, b.inode, b.endOff, b.fp)
				continue
			}
			if err := m.publishWithRetry(ctx, events); err != nil {
				m.setErr(b.task.ID, err.Error())
				// Requeue once: losing a batch silently is worse than a duplicate.
				select {
				case m.sendQ <- b:
				default:
				}
				time.Sleep(time.Second)
				continue
			}
			m.reg.Advance(b.task.ID, b.path, b.inode, b.endOff, b.fp)
			atomic.AddInt64(&statsOf(b.task.ID).eventsSent, int64(len(events)))
			atomic.AddInt64(&statsOf(b.task.ID).bytesRead, b.endOff)
			m.setErr(b.task.ID, "")
		}
	}
}

func (m *Manager) applyProcessors(taskID string, events []Event) []Event {
	m.mu.RLock()
	rt, ok := m.tasks[taskID]
	m.mu.RUnlock()
	if !ok || len(rt.procs) == 0 {
		return events
	}
	out := make([]Event, 0, len(events))
	for _, ev := range events {
		cur := []Event{ev}
		for _, p := range rt.procs {
			if len(cur) == 0 {
				break
			}
			var next []Event
			for _, e := range cur {
				next = append(next, p.Apply(e)...)
			}
			cur = next
		}
		out = append(out, cur...)
	}
	if dropped := len(events) - len(out); dropped > 0 {
		atomic.AddInt64(&statsOf(taskID).eventsDropped, int64(dropped))
	}
	return out
}

// publishWithRetry backs off instead of dropping. The queue in front of it is
// bounded, so a long outage applies back-pressure to the readers — logs pile up
// on disk where they belong, not in memory.
func (m *Manager) publishWithRetry(ctx context.Context, events []Event) error {
	delay := time.Second
	for attempt := 0; attempt < 5; attempt++ {
		err := m.out.Publish(ctx, events)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return err
		case <-m.stop:
			return err
		case <-time.After(delay):
		}
		if delay < 30*time.Second {
			delay *= 2
		}
	}
	return m.out.Publish(ctx, events)
}

// Stop shuts the pipeline down.
func (m *Manager) Stop() {
	select {
	case <-m.stop:
	default:
		close(m.stop)
	}
}

func (m *Manager) stopAll() {
	m.mu.Lock()
	m.tasks = map[string]*taskRuntime{}
	m.mu.Unlock()
}

func (m *Manager) setErr(taskID, msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rt, ok := m.tasks[taskID]; ok {
		if msg == "" {
			rt.state = "running"
			rt.task.Warnings = nil
		} else {
			rt.state = "failed"
			rt.task.Warnings = []string{msg}
		}
	}
}

// Status returns everything the panel needs in one call.
func (m *Manager) Status() PipelineStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	st := PipelineStatus{
		Enabled:   m.cfg.Enabled,
		TasksDir:  m.cfg.TasksDir,
		Output:    m.cfg.Output.Type,
		OutputErr: m.outErr,
		Warnings:  append([]string{}, m.warnings...),
		LoadedAt:  m.loadedAt,
		Tasks:     make([]TaskStatus, 0, len(m.tasks)),
	}
	if m.out != nil {
		st.Stats = m.out.Stats()
	}
	ids := make([]string, 0, len(m.tasks))
	for id := range m.tasks {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		rt := m.tasks[id]
		s := statsOf(id)
		lastErr := ""
		if rt.state == "failed" && len(rt.task.Warnings) > 0 {
			lastErr = rt.task.Warnings[0]
		}
		st.Tasks = append(st.Tasks, TaskStatus{
			TaskID:        id,
			File:          filepath.Base(rt.task.File),
			Paths:         rt.task.Input.Paths,
			Output:        m.cfg.Output.Type,
			State:         rt.state,
			FilesOpen:     int(atomic.LoadInt64(&s.filesOpen)),
			BytesRead:     atomic.LoadInt64(&s.bytesRead),
			EventsSent:    atomic.LoadInt64(&s.eventsSent),
			EventsDropped: atomic.LoadInt64(&s.eventsDropped),
			LastError:     lastErr,
			UpdatedAt:     rt.task.LoadedAt,
		})
	}
	return st
}

// ReloadNow forces an immediate rescan of tasks.d (panel button / LogTask apply).
func (m *Manager) ReloadNow() { m.reload() }

// Config returns the active configuration (read-only use).
func (m *Manager) Config() MainConfig { return m.cfg }

// Metrics renders opsmini_log_* series for /metrics.
func (m *Manager) Metrics() string {
	st := m.Status()
	var b strings.Builder
	b.WriteString("# HELP opsmini_log_pipeline_tasks Number of loaded collection tasks.\n")
	b.WriteString("# TYPE opsmini_log_pipeline_tasks gauge\n")
	fmt.Fprintf(&b, "opsmini_log_pipeline_tasks %d\n", len(st.Tasks))
	b.WriteString("# HELP opsmini_log_events_sent_total Events accepted by the output.\n")
	b.WriteString("# TYPE opsmini_log_events_sent_total counter\n")
	for _, t := range st.Tasks {
		fmt.Fprintf(&b, "opsmini_log_events_sent_total{task=%q} %d\n", t.TaskID, t.EventsSent)
		fmt.Fprintf(&b, "opsmini_log_events_dropped_total{task=%q} %d\n", t.TaskID, t.EventsDropped)
		fmt.Fprintf(&b, "opsmini_log_bytes_read_total{task=%q} %d\n", t.TaskID, t.BytesRead)
	}
	return b.String()
}

// expandPaths turns the task globs into a concrete file list.
func expandPaths(patterns, excludes []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range patterns {
		matches, err := filepath.Glob(p)
		if err != nil {
			continue
		}
		for _, m := range matches {
			if seen[m] || excluded(excludes, m) {
				continue
			}
			if st, err := os.Stat(m); err != nil || st.IsDir() {
				continue
			}
			seen[m] = true
			out = append(out, m)
		}
	}
	sort.Strings(out)
	return out
}
