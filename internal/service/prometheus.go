package service

import (
	"fmt"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
)

// PrometheusMetrics returns system metrics in Prometheus text format.
// Metric naming and labels are aligned with node_exporter so the Prometheus
// community Node Dashboard can be used directly.
func (s *SystemService) PrometheusMetrics() string {
	var b strings.Builder
	b.WriteString("# OpsMini node metrics (node_exporter compatible)\n")

	// CPU per-core per-mode cumulative seconds (counter)
	if times, err := cpu.Times(true); err == nil && len(times) > 0 {
		b.WriteString("# HELP node_cpu_seconds_total Seconds the CPUs spent in each mode.\n")
		b.WriteString("# TYPE node_cpu_seconds_total counter\n")
		modes := []struct {
			key string
			val float64
		}{}
		for i, t := range times {
			modes = append(modes, []struct {
				key string
				val float64
			}{
				{"idle", t.Idle}, {"user", t.User}, {"system", t.System},
				{"nice", t.Nice}, {"iowait", t.Iowait}, {"irq", t.Irq},
				{"softirq", t.Softirq}, {"steal", t.Steal},
			}...)
			for _, m := range modes {
				fmt.Fprintf(&b, "node_cpu_seconds_total{cpu=\"%d\",mode=\"%s\"} %.2f\n", i, m.key, m.val)
			}
			modes = modes[:0]
		}
	}

	// memory (bytes, gauge)
	if vm, err := mem.VirtualMemory(); err == nil {
		b.WriteString("# HELP node_memory_MemTotal_bytes Memory information field MemTotal_bytes.\n")
		b.WriteString("# TYPE node_memory_MemTotal_bytes gauge\n")
		fmt.Fprintf(&b, "node_memory_MemTotal_bytes %.0f\n", float64(vm.Total))
		fmt.Fprintf(&b, "node_memory_MemFree_bytes %.0f\n", float64(vm.Free))
		fmt.Fprintf(&b, "node_memory_MemAvailable_bytes %.0f\n", float64(vm.Available))
		fmt.Fprintf(&b, "node_memory_Buffers_bytes %.0f\n", float64(vm.Buffers))
		fmt.Fprintf(&b, "node_memory_Cached_bytes %.0f\n", float64(vm.Cached))
		fmt.Fprintf(&b, "node_memory_used_percent %.2f\n", vm.UsedPercent)
	}

	// disk partitions (bytes + usage)
	if parts, err := disk.Partitions(false); err == nil {
		b.WriteString("# HELP node_filesystem_size_bytes Filesystem size in bytes.\n")
		b.WriteString("# TYPE node_filesystem_size_bytes gauge\n")
		for _, p := range parts {
			if u, err := disk.Usage(p.Mountpoint); err == nil {
				mp := escapeLabel(p.Mountpoint)
				fmt.Fprintf(&b, "node_filesystem_size_bytes{mountpoint=\"%s\",fstype=\"%s\"} %.0f\n", mp, p.Fstype, float64(u.Total))
				fmt.Fprintf(&b, "node_filesystem_avail_bytes{mountpoint=\"%s\",fstype=\"%s\"} %.0f\n", mp, p.Fstype, float64(u.Free))
				fmt.Fprintf(&b, "node_filesystem_used_percent{mountpoint=\"%s\"} %.2f\n", mp, u.UsedPercent)
			}
		}
	}

	// network interface cumulative traffic (counter, bytes)
	if counters, err := gnet.IOCounters(true); err == nil {
		b.WriteString("# HELP node_network_receive_bytes_total Network device statistic receive_bytes.\n")
		b.WriteString("# TYPE node_network_receive_bytes_total counter\n")
		for _, c := range counters {
			fmt.Fprintf(&b, "node_network_receive_bytes_total{device=\"%s\"} %.0f\n", c.Name, float64(c.BytesRecv))
			fmt.Fprintf(&b, "node_network_transmit_bytes_total{device=\"%s\"} %.0f\n", c.Name, float64(c.BytesSent))
		}
	}

	// load (1/5/15 minutes)
	if l, err := load.Avg(); err == nil {
		b.WriteString("# HELP node_load1 1m load average.\n")
		b.WriteString("# TYPE node_load1 gauge\n")
		fmt.Fprintf(&b, "node_load1 %.2f\nnode_load5 %.2f\nnode_load15 %.2f\n", l.Load1, l.Load5, l.Load15)
	}

	// host information and time
	if info, err := host.Info(); err == nil {
		b.WriteString("# HELP node_uname_info Labeled system information.\n")
		b.WriteString("# TYPE node_uname_info gauge\n")
		fmt.Fprintf(&b, "node_uname_info{nodename=\"%s\",sysname=\"%s\",release=\"%s\",machine=\"%s\"} 1\n",
			escapeLabel(info.Hostname), escapeLabel(info.OS), escapeLabel(info.KernelVersion), escapeLabel(info.KernelArch))
		fmt.Fprintf(&b, "node_boot_time_seconds %.0f\n", float64(info.BootTime))
	}
	fmt.Fprintf(&b, "node_time_seconds %.0f\n", float64(time.Now().Unix()))

	return b.String()
}

// escapeLabel escapes special characters in a Prometheus label value.
func escapeLabel(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n")
	return r.Replace(s)
}
