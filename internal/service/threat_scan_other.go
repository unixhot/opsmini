//go:build !windows

package service

import "github.com/opsmini/opsmini/internal/model"

// scanAll runs the Unix threat checks: process table, crontab and rc.local.
func (s *ThreatService) scanAll() []model.ThreatFinding {
	findings := append(s.scanProcesses(), s.scanCron()...)
	return append(findings, s.scanStartup()...)
}
