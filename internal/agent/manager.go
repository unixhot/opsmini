package agent

import (
	"context"
	"log"
	"sync"
)

// Manager manages the Agent gRPC client lifecycle so the connection
// configuration (OpsAnt "OpsMini Server" address + token) can be changed
// at runtime from the panel's settings page without restarting the process.
type Manager struct {
	mu         sync.Mutex
	cfg        Config
	hostname   string
	version    string
	instanceID string
	cancel     context.CancelFunc
	running    bool
	// ctrl is carried across reconnects so a restart of the link keeps the
	// subsystems wired to the new client.
	ctrl controls
}

// NewManager creates an Agent lifecycle manager. instanceID is the panel's
// persisted instance ID (opsmini-xxxxxxxxxxxx) reported on every registration.
func NewManager(hostname, version, instanceID string) *Manager {
	return &Manager{hostname: hostname, version: version, instanceID: instanceID}
}

// Start (re)starts the Agent with the given config. An empty ServerAddr
// stops any currently running Agent (disconnect from OpsAnt).
func (m *Manager) Start(cfg Config) {
	m.mu.Lock()
	defer m.mu.Unlock()
	wasRunning := m.running
	prevAddr := m.cfg.ServerAddr
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.running = false
	m.cfg = cfg
	if cfg.ServerAddr == "" {
		// Log the transition explicitly: otherwise a manual disconnect looks
		// identical to a network drop in the journal, and nobody can tell
		// whether the operator turned it off or OpsAnt went away.
		if wasRunning {
			log.Printf("agent: stopped, server address cleared (was %s) — manual disconnect", prevAddr)
		}
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	cl := New(cfg, m.hostname, m.version, m.instanceID)
	cl.SetControls(m.ctrl.plugin)
	go cl.Run(ctx)
	m.cancel = cancel
	m.running = true
}

// Stop disconnects the Agent if it is running.
func (m *Manager) Stop() {
	m.Start(Config{})
}

// Identity returns the agent's base identity (hostname, version, instance ID).
// A connectivity probe derives a throwaway identity from these so the values
// stay recognisable in the OpsAnt host list without colliding with the live
// agent's registration.
func (m *Manager) Identity() (hostname, version, instanceID string) {
	return m.hostname, m.version, m.instanceID
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
