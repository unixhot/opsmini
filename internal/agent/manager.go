package agent

import (
	"context"
	"sync"
)

// Manager manages the Agent gRPC client lifecycle so the connection
// configuration (OpsAnt "OpsMini Server" address + token) can be changed
// at runtime from the panel's settings page without restarting the process.
type Manager struct {
	mu       sync.Mutex
	cfg      Config
	hostname string
	version  string
	cancel   context.CancelFunc
	running  bool
}

// NewManager creates an Agent lifecycle manager.
func NewManager(hostname, version string) *Manager {
	return &Manager{hostname: hostname, version: version}
}

// Start (re)starts the Agent with the given config. An empty ServerAddr
// stops any currently running Agent (disconnect from OpsAnt).
func (m *Manager) Start(cfg Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.running = false
	m.cfg = cfg
	if cfg.ServerAddr == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	go New(cfg, m.hostname, m.version).Run(ctx)
	m.cancel = cancel
	m.running = true
}

// Stop disconnects the Agent if it is running.
func (m *Manager) Stop() {
	m.Start(Config{})
}

// Config returns the current effective config.
func (m *Manager) Config() Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cfg
}

// Running reports whether the Agent is currently running.
func (m *Manager) Running() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}
