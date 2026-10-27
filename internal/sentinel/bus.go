package sentinel

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// Bus de-duplicates, persists and fans out signals.
//
// Every collector publishes here; consumers (the incident aggregator, the UI
// stream) subscribe. De-duplication happens on DedupKey so a chatty source (for
// example a log file growing for an hour) produces one signal per window
// instead of thousands.
type Bus struct {
	db      *gorm.DB
	in      chan model.Signal
	window  time.Duration
	mu      sync.Mutex
	seen    map[string]time.Time
	subs    []func(model.Signal)
	dropped int
}

// NewBus creates a bus with the given de-duplication window.
func NewBus(db *gorm.DB, window time.Duration) *Bus {
	if window <= 0 {
		window = 30 * time.Minute
	}
	return &Bus{
		db:     db,
		in:     make(chan model.Signal, 256),
		window: window,
		seen:   make(map[string]time.Time),
	}
}

// Subscribe registers a synchronous handler (called from the bus goroutine, so
// keep it fast).
func (b *Bus) Subscribe(fn func(model.Signal)) {
	b.mu.Lock()
	b.subs = append(b.subs, fn)
	b.mu.Unlock()
}

// Publish enqueues a signal. It never blocks: when the buffer is full the signal
// is dropped and counted, which is preferable to stalling a collector.
func (b *Bus) Publish(s model.Signal) {
	select {
	case b.in <- s:
	default:
		b.mu.Lock()
		b.dropped++
		b.mu.Unlock()
	}
}

// Dropped reports how many signals were dropped due to back-pressure.
func (b *Bus) Dropped() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.dropped
}

// Run consumes the queue until ctx is cancelled.
func (b *Bus) Run(ctx context.Context) {
	sweep := time.NewTicker(10 * time.Minute)
	defer sweep.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			b.sweep()
		case s := <-b.in:
			b.handle(s)
		}
	}
}

func (b *Bus) handle(s model.Signal) {
	key := s.DedupKey
	if key == "" {
		key = s.Fingerprint + "|" + s.Subject
	}
	now := time.Now()

	b.mu.Lock()
	for k, t := range b.seen {
		if now.Sub(t) > b.window {
			delete(b.seen, k)
		}
	}
	if last, ok := b.seen[key]; ok && now.Sub(last) < b.window {
		b.mu.Unlock()
		return
	}
	b.seen[key] = now
	subs := append([]func(model.Signal){}, b.subs...)
	b.mu.Unlock()

	if b.db != nil {
		if err := b.db.Create(&s).Error; err != nil {
			log.Printf("sentinel: persist signal: %v", err)
		}
	}
	for _, fn := range subs {
		fn(s)
	}
}

// sweep drops de-duplication entries and old signals.
func (b *Bus) sweep() {
	b.mu.Lock()
	now := time.Now()
	for k, t := range b.seen {
		if now.Sub(t) > b.window {
			delete(b.seen, k)
		}
	}
	b.mu.Unlock()
	if b.db != nil {
		cut := time.Now().Add(-7 * 24 * time.Hour)
		b.db.Where("observed_at < ?", cut).Delete(&model.Signal{})
	}
}

// --- incident aggregation ----------------------------------------------------

// Aggregator folds signals that share a family and subject inside a time window
// into one Incident, raising its severity instead of creating noise.
type Aggregator struct {
	db     *gorm.DB
	window time.Duration
	mu     sync.Mutex
	subs   []func(model.Incident, model.Signal)
}

// NewAggregator creates an aggregator with the given grouping window.
func NewAggregator(db *gorm.DB, window time.Duration) *Aggregator {
	if window <= 0 {
		window = 10 * time.Minute
	}
	return &Aggregator{db: db, window: window}
}

// OnIncident registers a handler called whenever an incident is created or updated.
func (a *Aggregator) OnIncident(fn func(model.Incident, model.Signal)) {
	a.mu.Lock()
	a.subs = append(a.subs, fn)
	a.mu.Unlock()
}

// Add folds one signal into an incident and returns it. The boolean reports
// whether the incident was created by this call.
func (a *Aggregator) Add(s model.Signal) (model.Incident, bool) {
	if a.db == nil {
		return model.Incident{}, false
	}
	var inc model.Incident
	cut := time.Now().Add(-a.window)
	err := a.db.Where(
		"family = ? AND subject = ? AND status IN ? AND last_seen > ?",
		s.Family, s.Subject,
		[]string{model.IncidentOpen, model.IncidentTriggered, model.IncidentRunning},
		cut,
	).Order("last_seen DESC").First(&inc).Error

	created := false
	if err != nil {
		inc = model.Incident{
			ID:        newIncidentID(s.ID),
			Title:     titleFor(s),
			Family:    s.Family,
			Subject:   s.Subject,
			Severity:  s.Severity,
			Status:    model.IncidentOpen,
			Count:     1,
			FirstSeen: s.ObservedAt,
			LastSeen:  s.ObservedAt,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := a.db.Create(&inc).Error; err != nil {
			log.Printf("sentinel: create incident: %v", err)
			return inc, false
		}
		created = true
	} else {
		inc.Count++
		inc.LastSeen = s.ObservedAt
		if model.SeverityRank(s.Severity) > model.SeverityRank(inc.Severity) {
			inc.Severity = s.Severity
		}
		inc.UpdatedAt = time.Now()
		if err := a.db.Model(&model.Incident{}).Where("id = ?", inc.ID).
			Updates(map[string]any{
				"count":      inc.Count,
				"last_seen":  inc.LastSeen,
				"severity":   inc.Severity,
				"updated_at": inc.UpdatedAt,
			}).Error; err != nil {
			log.Printf("sentinel: update incident: %v", err)
		}
	}

	// Link the signal to the incident.
	a.db.Model(&model.Signal{}).Where("id = ?", s.ID).Update("incident_id", inc.ID)

	a.mu.Lock()
	subs := append([]func(model.Incident, model.Signal){}, a.subs...)
	a.mu.Unlock()
	for _, fn := range subs {
		fn(inc, s)
	}
	return inc, created
}

func newIncidentID(seed string) string {
	if len(seed) >= 32 {
		return seed[:32]
	}
	return seed + strings.Repeat("0", 32-len(seed))
}

func titleFor(s model.Signal) string {
	switch s.Family {
	case "log_growth":
		return "日志异常增长 · " + s.Subject
	case "auth.anomaly":
		return "登录行为异常 · " + s.Subject
	case "webshell":
		return "疑似 WebShell · " + s.Subject
	case "file.integrity":
		return "关键文件变更 · " + s.Subject
	case "proc.anomaly":
		return "可疑进程 · " + s.Subject
	case "net.anomaly":
		return "异常外联 · " + s.Subject
	case "metric":
		return "指标告警 · " + s.Subject
	}
	return s.Family + " · " + s.Subject
}
