// Package exporter exposes the host's metrics in windows_exporter's format so a
// host that used to run the standalone exporter can point Prometheus at the panel
// instead, without changing dashboards or alert rules.
package exporter

// Collector renders Prometheus text format for a set of collectors.
type Collector interface {
	// Render scrapes every collector and returns the text exposition.
	Render() ([]byte, error)
	// CollectorNames returns the collectors that built successfully.
	CollectorNames() []string
	// Close releases the collectors and their WMI session.
	Close() error
}
