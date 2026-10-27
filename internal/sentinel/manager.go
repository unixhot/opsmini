package sentinel

import (
	"context"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// Config controls the perception layer. It is built from panel settings so the
// administrator can tune it without restarting the panel.
type Config struct {
	Enabled  bool
	Learning bool // learning period: record everything, trigger nothing
	DataDir  string

	// LogPaths are the roots scanned for growing log files.
	LogPaths []string
	// WebRoots are the roots watched for new/changed scripts (webshell landing).
	WebRoots []string
	// OffHoursStart/End bound the "off hours" window (local time).
	OffHoursStart int
	OffHoursEnd   int

	// Thresholds.
	LogGrowthBytesPerMin int64
	LogGrowthSigma       float64
	AuthFailBurst        int

	// Interval for the fast collectors.
	PollInterval time.Duration
}

// DefaultConfig returns the shipped defaults.
func DefaultConfig() Config {
	return Config{
		Enabled:              true,
		Learning:             false,
		DataDir:              "/var/lib/opsmini/data",
		LogPaths:             []string{"/var/log"},
		WebRoots:             []string{"/var/www", "/var/www/html", "/www/wwwroot", "/data/www"},
		OffHoursStart:        0,
		OffHoursEnd:          6,
		LogGrowthBytesPerMin: 20 << 20,
		LogGrowthSigma:       3,
		AuthFailBurst:        20,
		PollInterval:         2 * time.Second,
	}
}

// Manager owns the collectors, the bus and the aggregator.
type Manager struct {
	cfg       Config
	db        *gorm.DB
	Bus       *Bus
	Agg       *Aggregator
	collectors []collector
	stop      chan struct{}
	wg        sync.WaitGroup
	startedAt time.Time
}

// collector is one perception source.
type collector interface {
	Name() string
	Run(ctx context.Context, out func(Signal))
}

// New builds the perception layer.
func New(cfg Config, db *gorm.DB) *Manager {
	bus := NewBus(db, 30*time.Minute)
	agg := NewAggregator(db, 10*time.Minute)
	m := &Manager{cfg: cfg, db: db, Bus: bus, Agg: agg, stop: make(chan struct{})}

	// Wire the fan-out: bus -> aggregator.
	bus.Subscribe(func(s model.Signal) {
		agg.Add(s)
	})

	m.collectors = []collector{
		newLogWatcher(cfg),
		newAuthWatcher(cfg, db),
		newFileWatcher(cfg),
		newProcWatcher(cfg),
		newNetWatcher(cfg),
	}
	return m
}

// NewWithMetricHook adds the metric collector on top of the defaults. The metric
// collector is fed by the existing alert engine instead of polling on its own.
func (m *Manager) NewWithMetricHook(fire chan model.Signal) {
	m.collectors = append(m.collectors, &metricCollector{in: fire})
}

// Config returns the active configuration.
func (m *Manager) Config() Config { return m.cfg }

// StartedAt reports when the manager started.
func (m *Manager) StartedAt() time.Time { return m.startedAt }

// Start launches the bus and every collector.
func (m *Manager) Start(ctx context.Context) {
	if !m.cfg.Enabled {
		log.Printf("sentinel: disabled by configuration")
		return
	}
	m.startedAt = time.Now()
	go m.Bus.Run(ctx)
	for _, c := range m.collectors {
		c := c
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			log.Printf("sentinel: collector %s started", c.Name())
			c.Run(ctx, func(sig Signal) { m.Bus.Publish(sig.Signal) })
		}()
	}
	log.Printf("sentinel: perception layer up (log_paths=%v web_roots=%v learning=%v)",
		m.cfg.LogPaths, m.cfg.WebRoots, m.cfg.Learning)
}

// Stop waits for the collectors to return (ctx cancellation drives them).
func (m *Manager) Stop() {
	close(m.stop)
	m.wg.Wait()
}

// Stats is a small snapshot for the UI.
type Stats struct {
	Enabled    bool      `json:"enabled"`
	Learning   bool      `json:"learning"`
	StartedAt  time.Time `json:"started_at"`
	Collectors []string  `json:"collectors"`
	Dropped    int       `json:"dropped"`
}

// Stats returns the current perception state.
func (m *Manager) Stats() Stats {
	names := make([]string, 0, len(m.collectors))
	for _, c := range m.collectors {
		names = append(names, c.Name())
	}
	return Stats{
		Enabled:    m.cfg.Enabled,
		Learning:   m.cfg.Learning,
		StartedAt:  m.startedAt,
		Collectors: names,
		Dropped:    m.Bus.Dropped(),
	}
}
