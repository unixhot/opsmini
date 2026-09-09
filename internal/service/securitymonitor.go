package service

import (
	"log"
	"time"
)

// SecurityMonitor runs the host security loop: FIM comparison + threat scanning.
type SecurityMonitor struct {
	fim    *FimService
	threat *ThreatService
	stopCh chan struct{}
}

// NewSecurityMonitor creates a SecurityMonitor.
func NewSecurityMonitor(fim *FimService, threat *ThreatService) *SecurityMonitor {
	return &SecurityMonitor{fim: fim, threat: threat, stopCh: make(chan struct{})}
}

// Start starts the background detection loop.
func (m *SecurityMonitor) Start() {
	go m.loop()
}

// Stop stops the background detection loop.
func (m *SecurityMonitor) Stop() {
	select {
	case <-m.stopCh:
	default:
		close(m.stopCh)
	}
}

// loop runs the periodic security checks: FIM check and threat scan.
func (m *SecurityMonitor) loop() {
	// run once on startup
	m.fim.Check()
	if _, err := m.threat.Scan(); err != nil {
		log.Printf("security: threat scan error: %v", err)
	}

	fimTicker := time.NewTicker(60 * time.Second)
	threatTicker := time.NewTicker(5 * time.Minute)
	defer fimTicker.Stop()
	defer threatTicker.Stop()

	for {
		select {
		case <-m.stopCh:
			return
		case <-fimTicker.C:
			if changes := m.fim.Check(); len(changes) > 0 {
				log.Printf("security: %d critical file changes detected", len(changes))
			}
		case <-threatTicker.C:
			if _, err := m.threat.Scan(); err != nil {
				log.Printf("security: threat scan error: %v", err)
			}
		}
	}
}
