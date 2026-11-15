//go:build !windows

package store

// unsupportedAlertMetrics lists metric names that cannot be collected on this
// platform. Unix collects all of them, so the set is empty.
func unsupportedAlertMetrics() map[string]bool { return nil }
