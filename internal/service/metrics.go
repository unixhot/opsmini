// Package service provides background system metric collection and persistence.
package service

import (
	"log"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	"github.com/shirou/gopsutil/v4/net"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
)

// MetricPoint is a single sample point (used for dashboards/monitoring trend charts).
type MetricPoint struct {
	Time    int64   `json:"time"`     // unix seconds
	CPU     float64 `json:"cpu"`      // %
	Mem     float64 `json:"mem"`      // %
	Load1   float64 `json:"load1"`    // 1-minute load
	NetRecv float64 `json:"net_recv"` // bytes/s
	NetSent float64 `json:"net_sent"` // bytes/s
}

// MetricCollector is the background metric collector.
// Sample points are also persisted to SQLite (retained for 30 days); the in-memory
// buffer is only a fallback when no DB is available.
type MetricCollector struct {
	mu       sync.RWMutex
	points   []MetricPoint
	max      int
	interval time.Duration
	db       *gorm.DB

	stopCh   chan struct{}
	lastRecv uint64
	lastSent uint64
	lastAt   time.Time
}

// NewMetricCollector creates a collector. max is the in-memory buffer capacity,
// interval is the sampling period, and db is used for persistence (may be nil).
func NewMetricCollector(interval time.Duration, max int, db *gorm.DB) *MetricCollector {
	return &MetricCollector{
		points:   make([]MetricPoint, 0, max),
		max:      max,
		interval: interval,
		db:       db,
		stopCh:   make(chan struct{}),
	}
}

// Start starts background sampling. The first sample is taken immediately.
func (m *MetricCollector) Start() {
	// initial network baseline
	if io, err := net.IOCounters(false); err == nil && len(io) > 0 {
		m.lastRecv, m.lastSent = io[0].BytesRecv, io[0].BytesSent
		m.lastAt = time.Now()
	}
	go m.loop()
}

// Stop stops sampling.
func (m *MetricCollector) Stop() {
	close(m.stopCh)
}

// loop samples metrics at the configured interval until stopped, cleaning up old points hourly.
func (m *MetricCollector) loop() {
	cleanTicker := time.NewTicker(time.Hour)
	defer cleanTicker.Stop()
	for {
		m.sample()
		select {
		case <-m.stopCh:
			return
		case <-time.After(m.interval):
		case <-cleanTicker.C:
			m.cleanup()
		}
	}
}

// cleanup deletes persisted sample points older than 30 days to prevent unbounded database growth.
func (m *MetricCollector) cleanup() {
	if m.db == nil {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -30).Unix()
	res := m.db.Where("time < ?", cutoff).Delete(&model.MetricPoint{})
	if res.Error != nil {
		log.Printf("metric cleanup: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("metric cleanup: removed %d points older than 30 days", res.RowsAffected)
	}
}

// sample collects one metric point (CPU/memory/disk/network) and persists it.
func (m *MetricCollector) sample() {
	// CPU percentage uses blocking sampling; interval is the observation window.
	cpuPerc, _ := cpu.Percent(m.interval, false)
	var cpuVal float64
	if len(cpuPerc) > 0 {
		cpuVal = cpuPerc[0]
	}

	vm, _ := mem.VirtualMemory()
	var memVal float64
	if vm != nil {
		memVal = vm.UsedPercent
	}

	lavg, _ := load.Avg()
	var load1 float64
	if lavg != nil {
		load1 = lavg.Load1
	}

	// network rate (cumulative byte difference / time)
	var recvRate, sentRate float64
	if io, err := net.IOCounters(false); err == nil && len(io) > 0 {
		now := time.Now()
		elapsed := now.Sub(m.lastAt).Seconds()
		if elapsed > 0 {
			recvRate = float64(io[0].BytesRecv-m.lastRecv) / elapsed
			sentRate = float64(io[0].BytesSent-m.lastSent) / elapsed
		}
		m.lastRecv, m.lastSent = io[0].BytesRecv, io[0].BytesSent
		m.lastAt = now
	}

	p := MetricPoint{
		Time:    time.Now().Unix(),
		CPU:     cpuVal,
		Mem:     memVal,
		Load1:   load1,
		NetRecv: recvRate,
		NetSent: sentRate,
	}

	m.mu.Lock()
	if len(m.points) >= m.max {
		// drop the oldest, keep it ring-buffered
		copy(m.points, m.points[1:])
		m.points[len(m.points)-1] = p
	} else {
		m.points = append(m.points, p)
	}
	m.mu.Unlock()

	// persist to SQLite (one row per 10s, negligible write pressure; failure only logs and does not affect collection)
	if m.db != nil {
		if err := m.db.Create(&model.MetricPoint{
			Time:     p.Time,
			CPU:      p.CPU,
			Mem:      p.Mem,
			Load1:    p.Load1,
			NetRecv:  p.NetRecv,
			NetSent:  p.NetSent,
		}).Error; err != nil {
			log.Printf("metric persist: %v", err)
		}
	}
}

// Snapshot returns a copy of the current buffer (ascending by time).
func (m *MetricCollector) Snapshot() []MetricPoint {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]MetricPoint, len(m.points))
	copy(out, m.points)
	return out
}

// Since returns the sample points within the last duration (no downsampling).
// It prefers querying SQLite for persisted data (survives restarts); falls back
// to the in-memory buffer when no DB is available.
func (m *MetricCollector) Since(d time.Duration) []MetricPoint {
	cutoff := time.Now().Add(-d).Unix()
	if m.db != nil {
		var rows []model.MetricPoint
		if err := m.db.Where("time >= ?", cutoff).Order("time ASC").Find(&rows).Error; err == nil && len(rows) > 0 {
			out := make([]MetricPoint, len(rows))
			for i, r := range rows {
				out[i] = MetricPoint{Time: r.Time, CPU: r.CPU, Mem: r.Mem, Load1: r.Load1, NetRecv: r.NetRecv, NetSent: r.NetSent}
			}
			return out
		}
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []MetricPoint
	for _, p := range m.points {
		if p.Time >= cutoff {
			out = append(out, p)
		}
	}
	return out
}

// Downsample downsamples points to at most limit points (using interval averages).
func Downsample(pts []MetricPoint, limit int) []MetricPoint {
	if limit <= 0 || len(pts) <= limit {
		return pts
	}
	bucket := (len(pts) + limit - 1) / limit
	out := make([]MetricPoint, 0, limit)
	for i := 0; i < len(pts); i += bucket {
		end := i + bucket
		if end > len(pts) {
			end = len(pts)
		}
		var p MetricPoint
		var n float64
		for _, v := range pts[i:end] {
			p.Time = v.Time
			p.CPU += v.CPU
			p.Mem += v.Mem
			p.Load1 += v.Load1
			p.NetRecv += v.NetRecv
			p.NetSent += v.NetSent
			n++
		}
		if n > 0 {
			p.CPU /= n
			p.Mem /= n
			p.Load1 /= n
			p.NetRecv /= n
			p.NetSent /= n
		}
		out = append(out, p)
	}
	return out
}
