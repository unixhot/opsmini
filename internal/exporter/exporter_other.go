//go:build !windows

package exporter

import "errors"

// Available reports whether the windows_exporter-compatible collectors are
// compiled in; they are Windows-only.
func Available() bool { return false }

// DefaultCollectors is empty off Windows.
func DefaultCollectors() string { return "" }

// Names lists every supported collector name; none off Windows.
func Names() []string { return nil }

// ParseList expands a comma-separated list; nothing applies off Windows.
func ParseList(string) []string { return nil }

// New always fails off Windows. Callers guard on Available.
func New([]string) (Collector, error) {
	return nil, errors.New("windows_exporter 兼容指标仅 Windows 可用")
}
