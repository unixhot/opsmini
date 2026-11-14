// Package windows_exporter assembles the vendored windows_exporter collectors
// into a single scrape that the OpsMini panel can serve.
//
// The collectors themselves are an unmodified copy of
// prometheus-community/windows_exporter v0.32.1 (MIT, see LICENSE-MIT). Only this
// file is ours: it owns the MI session, builds the requested collectors and
// renders Prometheus text format. The upstream framework package is not used
// because it hard-imports all fifty collectors.
package windows_exporter

import (
	"bytes"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/expfmt"

	"github.com/opsmini/opsmini/third_party/windows_exporter/mi"
	"github.com/opsmini/opsmini/third_party/windows_exporter/types"

	"github.com/opsmini/opsmini/third_party/windows_exporter/collector/cpu"
	"github.com/opsmini/opsmini/third_party/windows_exporter/collector/logical_disk"
	"github.com/opsmini/opsmini/third_party/windows_exporter/collector/memory"
	"github.com/opsmini/opsmini/third_party/windows_exporter/collector/net"
	"github.com/opsmini/opsmini/third_party/windows_exporter/collector/os"
	"github.com/opsmini/opsmini/third_party/windows_exporter/collector/physical_disk"
	"github.com/opsmini/opsmini/third_party/windows_exporter/collector/service"
	"github.com/opsmini/opsmini/third_party/windows_exporter/collector/system"
)

// DefaultCollectors is windows_exporter v0.32.1's [defaults] set. It is the
// compatibility contract: a host that scraped the standalone exporter gets the
// same metric families from the panel with no configuration change.
const DefaultCollectors = "cpu,memory,logical_disk,physical_disk,net,os,service,system"

// collector is the subset of the upstream Collector interface this file needs.
type collector interface {
	GetName() string
	Build(logger *slog.Logger, miSession *mi.Session) error
	Collect(ch chan<- prometheus.Metric, maxScrapeDuration time.Duration) error
	Close() error
}

// factories maps a collector name to its constructor. Adding a collector here is
// all it takes to enable it, because the vendored tree already contains it.
var factories = map[string]func() collector{
	cpu.Name:           func() collector { return cpu.New(nil) },
	memory.Name:        func() collector { return memory.New(nil) },
	logical_disk.Name:  func() collector { return logical_disk.New(nil) },
	physical_disk.Name: func() collector { return physical_disk.New(nil) },
	net.Name:           func() collector { return net.New(nil) },
	os.Name:            func() collector { return os.New(nil) },
	service.Name:       func() collector { return service.New(nil) },
	system.Name:        func() collector { return system.New(nil) },
}

// Exporter owns one MI session and the built collectors.
type Exporter struct {
	logger     *slog.Logger
	collectors []collector
	app        *mi.Application
	session    *mi.Session
	timeout    time.Duration

	mu     sync.Mutex
	closed bool
}

// Names returns the supported collector names, sorted.
func Names() []string {
	out := make([]string, 0, len(factories))
	for n := range factories {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ParseList accepts a comma-separated list, expanding "[defaults]".
func ParseList(s string) []string {
	s = strings.ReplaceAll(s, "[defaults]", DefaultCollectors)
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// New builds the requested collectors. An unknown name is an error so the caller
// can answer 400 the way the standalone exporter does.
func New(names []string) (*Exporter, error) {
	if len(names) == 0 {
		names = ParseList(DefaultCollectors)
	}
	logger := slog.New(slog.DiscardHandler)

	e := &Exporter{logger: logger, timeout: 20 * time.Second}

	// MI is only needed by collectors that read WMI classes (service, os, net...).
	// Failing to open it must not stop the PDH-only collectors from working, so
	// the error is deferred to Build: each collector reports what it needs.
	app, err := mi.ApplicationInitialize()
	if err == nil {
		e.app = app
		if opts, oerr := app.NewDestinationOptions(); oerr == nil {
			if serr := opts.SetLocale(mi.LocaleEnglish); serr == nil {
				if terr := opts.SetTimeout(time.Second); terr == nil {
					if sess, serr2 := app.NewSession(opts); serr2 == nil {
						e.session = sess
					}
				}
			}
			defer func() { _ = opts.Delete() }()
		}
	}

	for _, name := range names {
		factory, ok := factories[name]
		if !ok {
			e.Close()
			return nil, fmt.Errorf("unknown collector %q", name)
		}
		c := factory()
		if berr := c.Build(logger, e.session); berr != nil {
			// A collector that cannot build on this host is skipped rather than
			// failing the whole scrape: windows_exporter behaves the same way.
			e.logger.Warn("collector build failed", "collector", name, "err", berr)
			continue
		}
		e.collectors = append(e.collectors, c)
	}
	return e, nil
}

// Render scrapes every collector and returns Prometheus text format, including
// the exporter's own scrape metadata that upstream tooling expects.
func (e *Exporter) Render() ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil, fmt.Errorf("exporter closed")
	}

	reg := prometheus.NewRegistry()
	reg.MustRegister(&scrape{exp: e})

	fams, err := reg.Gather()
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	enc := expfmt.NewEncoder(&buf, expfmt.NewFormat(expfmt.TypeTextPlain))
	for _, f := range fams {
		if eerr := enc.Encode(f); eerr != nil {
			return nil, eerr
		}
	}
	return buf.Bytes(), nil
}

// CollectorNames returns the collectors that actually built.
func (e *Exporter) CollectorNames() []string {
	out := make([]string, 0, len(e.collectors))
	for _, c := range e.collectors {
		out = append(out, c.GetName())
	}
	return out
}

// Close releases the collectors and the MI session.
func (e *Exporter) Close() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return nil
	}
	e.closed = true
	for _, c := range e.collectors {
		_ = c.Close()
	}
	e.collectors = nil
	if e.session != nil {
		_ = e.session.Close()
	}
	if e.app != nil {
		_ = e.app.Close()
	}
	return nil
}

// scrape adapts the collectors to a prometheus.Collector and records the
// per-collector duration/success series the upstream framework emits.
type scrape struct {
	exp *Exporter

	duration *prometheus.Desc
	success  *prometheus.Desc
	total    *prometheus.Desc
}

func (s *scrape) descs() {
	if s.duration != nil {
		return
	}
	s.duration = prometheus.NewDesc(
		prometheus.BuildFQName(types.Namespace, "exporter", "collector_duration_seconds"),
		"windows_exporter: Duration of a collector scrape.", []string{"collector"}, nil)
	s.success = prometheus.NewDesc(
		prometheus.BuildFQName(types.Namespace, "exporter", "collector_success"),
		"windows_exporter: Whether a collector succeeded.", []string{"collector"}, nil)
	s.total = prometheus.NewDesc(
		prometheus.BuildFQName(types.Namespace, "exporter", "scrape_duration_seconds"),
		"windows_exporter: Duration of a scrape.", nil, nil)
}

// Describe intentionally sends nothing: the descriptor set depends on what the
// host supports, so this is an unchecked collector.
func (s *scrape) Describe(chan<- *prometheus.Desc) {}

func (s *scrape) Collect(ch chan<- prometheus.Metric) {
	s.descs()
	start := time.Now()
	for _, c := range s.exp.collectors {
		cstart := time.Now()
		err := c.Collect(ch, s.exp.timeout)
		ok := 1.0
		if err != nil {
			ok = 0
			s.exp.logger.Warn("collector scrape failed", "collector", c.GetName(), "err", err)
		}
		ch <- prometheus.MustNewConstMetric(s.duration, prometheus.GaugeValue,
			time.Since(cstart).Seconds(), c.GetName())
		ch <- prometheus.MustNewConstMetric(s.success, prometheus.GaugeValue, ok, c.GetName())
	}
	ch <- prometheus.MustNewConstMetric(s.total, prometheus.GaugeValue, time.Since(start).Seconds())
}
