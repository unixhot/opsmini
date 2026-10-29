package change

import (
	"context"
	"log"
	"sync"
	"time"
)

// source is one change provider.
type source interface {
	Name() string
	Run(ctx context.Context)
}

// Watcher orchestrates all change sources, persists what they report and fans the
// result out to the signal bus.
type Watcher struct {
	cfg     *Config
	store   *Store
	snap    *Snapshots
	onEvent func(Event) // fan-out to the signal bus (may be nil)

	events chan Event
	mu     sync.RWMutex
	live   map[string]bool // source name -> currently running
	cancel context.CancelFunc

	wg sync.WaitGroup
}

// NewWatcher builds a watcher. diffQuotaBytes bounds diff body storage (0 = no
// limit) — metadata keeps flowing even when the quota is exhausted.
func NewWatcher(cfg *Config, store *Store, snap *Snapshots, onEvent func(Event)) (*Watcher, error) {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	cfg.normalize()
	w := &Watcher{
		cfg:     cfg,
		store:   store,
		snap:    snap,
		onEvent: onEvent,
		events:  make(chan Event, 512),
		live:    make(map[string]bool),
	}
	return w, nil
}

// Start launches every enabled source and the persistence loop.
func (w *Watcher) Start(parent context.Context) {
	ctx, cancel := context.WithCancel(parent)
	w.cancel = cancel

	// Heal the chain anchor before any new row is appended: an upgraded database may
	// have been pruned by an older build and would otherwise report a broken chain.
	w.store.EnsureChainAnchor()

	// persistence + fan-out loop
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for {
			select {
			case <-ctx.Done():
				// drain quickly so in-flight events are not lost
				for {
					select {
					case ev := <-w.events:
						w.persist(ev)
					default:
						return
					}
				}
			case ev := <-w.events:
				w.persist(ev)
			}
		}
	}()

	var srcs []source
	if w.cfg.File.MaxFileSize > 0 {
		// The file source persists synchronously: it needs the final row id to
		// file its pre-change snapshot, and deduplication can change that id.
		srcs = append(srcs, newFileSource(w.cfg, w.store, w.snap, w.persistSync))
	}
	if w.cfg.Package.Enabled {
		srcs = append(srcs, newPackageSource(w.cfg, w.store, w.enqueue))
	}
	if w.cfg.Service.Enabled {
		srcs = append(srcs, newServiceSource(w.cfg, w.enqueue))
	}
	if w.cfg.Account.Enabled {
		srcs = append(srcs, newAccountSource(w.cfg, w.enqueue))
	}
	if w.cfg.Firewall.Enabled {
		srcs = append(srcs, newFirewallSource(w.cfg, w.enqueue))
	}
	if w.cfg.Cron.Enabled {
		srcs = append(srcs, newCronSource(w.cfg, w.enqueue))
	}

	for _, s := range srcs {
		s := s
		w.mu.Lock()
		w.live[s.Name()] = true
		w.mu.Unlock()
		w.wg.Add(1)
		go func() {
			defer w.wg.Done()
			// One source panicking must not take the others down: a broken
			// detector should degrade coverage, not blind the whole system.
			defer func() {
				if r := recover(); r != nil {
					log.Printf("change: source %s panicked: %v", s.Name(), r)
				}
				w.mu.Lock()
				w.live[s.Name()] = false
				w.mu.Unlock()
			}()
			s.Run(ctx)
		}()
	}
}

// Stop shuts everything down and waits briefly for the writer to drain.
func (w *Watcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
	done := make(chan struct{})
	go func() { w.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
	}
}

// enqueue is the callback handed to sources. It never blocks the source for
// long: a full queue drops the event rather than stalling detection.
func (w *Watcher) enqueue(ev Event) {
	select {
	case w.events <- ev:
	default:
		log.Printf("change: queue full, dropping %s %s", ev.Kind, ev.Object.Key)
	}
}

// persistSync writes one event synchronously and returns its final id.
func (w *Watcher) persistSync(ev Event) string {
	rec, err := w.store.Append(ev)
	if err != nil {
		log.Printf("change: persist %s %s: %v", ev.Kind, ev.Object.Key, err)
		return ""
	}
	if w.onEvent != nil {
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("change: fan-out panic: %v", r)
				}
			}()
			w.onEvent(ev)
		}()
	}
	return rec.ID
}

func (w *Watcher) persist(ev Event) {
	if _, err := w.store.Append(ev); err != nil {
		log.Printf("change: persist %s %s: %v", ev.Kind, ev.Object.Key, err)
		return
	}
	if w.onEvent != nil {
		// Fan-out uses a recover guard: a bug in the signal pipeline must not
		// stop change recording.
		func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("change: fan-out panic: %v", r)
				}
			}()
			w.onEvent(ev)
		}()
	}
}

// SourceStatus reports which sources are currently running (for the panel's
// health strip).
type SourceStatus struct {
	Name    string `json:"name"`
	Running bool   `json:"running"`
}

// Status returns per-source liveness.
func (w *Watcher) Status() []SourceStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	order := []string{"file", "package", "service", "account", "firewall", "cron"}
	out := make([]SourceStatus, 0, len(order))
	for _, n := range order {
		out = append(out, SourceStatus{Name: n, Running: w.live[n]})
	}
	return out
}

// Config exposes the effective config (read-only).
func (w *Watcher) Config() *Config { return w.cfg }

// Store exposes the store for the API layer.
func (w *Watcher) Store() *Store { return w.store }
