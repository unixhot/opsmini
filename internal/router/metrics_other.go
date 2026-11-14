//go:build !windows

package router

// windowsMetrics has nothing to add off Windows.
func windowsMetrics([]string) ([]byte, bool) { return nil, true }

// windowsExporterAvailable reports whether the compatibility layer is compiled in.
func windowsExporterAvailable() bool { return false }
