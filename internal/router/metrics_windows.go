//go:build windows

package router

import (
	"sync"

	"github.com/opsmini/opsmini/internal/exporter"
)

// defaultExporter holds the once-built default collector set. Building it costs
// a WMI (MI) session, so it is reused across scrapes rather than rebuilt per
// request.
var (
	expOnce sync.Once
	expDef  exporter.Collector
	expErr  error
)

// windowsMetrics renders the windows_exporter-compatible metric families.
//
// With no explicit collector list it reuses the cached default instance. With an
// explicit list it builds a throwaway instance: that path exists to honour a
// Prometheus `params: collect[]` filter, which is rare enough not to warrant a
// cache. ok is false when a requested collector name is unknown, which the
// caller answers with 400 — the same contract as the standalone exporter.
func windowsMetrics(collect []string) (body []byte, ok bool) {
	if len(collect) == 0 {
		expOnce.Do(func() {
			expDef, expErr = exporter.New(nil)
		})
		if expErr != nil || expDef == nil {
			return nil, true
		}
		b, err := expDef.Render()
		if err != nil {
			return nil, true
		}
		return b, true
	}

	inst, err := exporter.New(collect)
	if err != nil {
		return nil, false
	}
	defer func() { _ = inst.Close() }()
	b, rerr := inst.Render()
	if rerr != nil {
		return nil, true
	}
	return b, true
}

// windowsExporterAvailable reports whether the compatibility layer is compiled in.
func windowsExporterAvailable() bool { return exporter.Available() }
