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
	"github.com/shirou/gopsutil/v4/process"
	"github.com/shirou/gopsutil/v4/sensors"
)

// PrometheusMetrics returns system metrics in Prometheus text format.
// Metric naming and labels are aligned with node_exporter so the Prometheus
// community Node Dashboard can be used directly. This is effectively an
// embedded subset of the node_exporter collectors (cpu/meminfo/filesystem/
// diskstats/netdev/loadavg/uname/time/vmstat/netstat/thermal).
func (s *SystemService) PrometheusMetrics() string {
	var b strings.Builder
	b.WriteString("# OpsMini node metrics (node_exporter compatible)\n")

	writeUsageMetrics(&b)
	writeCPUMetrics(&b)
	writeMemoryMetrics(&b)
	writeFilesystemMetrics(&b)
	writeDiskIOMetrics(&b)
	writeNetworkMetrics(&b)
	writeLoadMetrics(&b)
	writeProcMetrics(&b)
	writeNetstatMetrics(&b)
	writeThermalMetrics(&b)
	writeHostMetrics(&b)

	return b.String()
}

// writeUsageMetrics emits immediate CPU/memory usage percentages as gauges, so
// upstream tools (e.g. OpsAnt) can read a single sample without computing rates
// from the cumulative node_cpu_seconds_total counter.
func writeUsageMetrics(b *strings.Builder) {
	b.WriteString("# HELP opsmini_cpu_usage_percent Current CPU usage percentage (0-100).\n")
	b.WriteString("# TYPE opsmini_cpu_usage_percent gauge\n")
	if pcts, err := cpu.Percent(200*time.Millisecond, false); err == nil && len(pcts) > 0 {
		fmt.Fprintf(b, "opsmini_cpu_usage_percent %.2f\n", pcts[0])
	} else {
		fmt.Fprintf(b, "opsmini_cpu_usage_percent 0\n")
	}

	b.WriteString("# HELP opsmini_mem_usage_percent Current memory usage percentage (0-100).\n")
	b.WriteString("# TYPE opsmini_mem_usage_percent gauge\n")
	if vm, err := mem.VirtualMemory(); err == nil && vm != nil {
		fmt.Fprintf(b, "opsmini_mem_usage_percent %.2f\n", vm.UsedPercent)
	} else {
		fmt.Fprintf(b, "opsmini_mem_usage_percent 0\n")
	}
}

// writeCPUMetrics emits node_cpu_seconds_total (per-core per-mode cumulative seconds).
func writeCPUMetrics(b *strings.Builder) {
	times, err := cpu.Times(true)
	if err != nil || len(times) == 0 {
		return
	}
	b.WriteString("# HELP node_cpu_seconds_total Seconds the CPUs spent in each mode.\n")
	b.WriteString("# TYPE node_cpu_seconds_total counter\n")
	for i, t := range times {
		fmt.Fprintf(b, "node_cpu_seconds_total{cpu=\"%d\",mode=\"idle\"} %.2f\n", i, t.Idle)
		fmt.Fprintf(b, "node_cpu_seconds_total{cpu=\"%d\",mode=\"user\"} %.2f\n", i, t.User)
		fmt.Fprintf(b, "node_cpu_seconds_total{cpu=\"%d\",mode=\"system\"} %.2f\n", i, t.System)
		fmt.Fprintf(b, "node_cpu_seconds_total{cpu=\"%d\",mode=\"nice\"} %.2f\n", i, t.Nice)
		fmt.Fprintf(b, "node_cpu_seconds_total{cpu=\"%d\",mode=\"iowait\"} %.2f\n", i, t.Iowait)
		fmt.Fprintf(b, "node_cpu_seconds_total{cpu=\"%d\",mode=\"irq\"} %.2f\n", i, t.Irq)
		fmt.Fprintf(b, "node_cpu_seconds_total{cpu=\"%d\",mode=\"softirq\"} %.2f\n", i, t.Softirq)
		fmt.Fprintf(b, "node_cpu_seconds_total{cpu=\"%d\",mode=\"steal\"} %.2f\n", i, t.Steal)
	}
}

// writeMemoryMetrics emits node_memory_* (physical memory and swap).
func writeMemoryMetrics(b *strings.Builder) {
	vm, err := mem.VirtualMemory()
	if err == nil {
		b.WriteString("# HELP node_memory_MemTotal_bytes Memory information field MemTotal_bytes.\n")
		b.WriteString("# TYPE node_memory_MemTotal_bytes gauge\n")
		fmt.Fprintf(b, "node_memory_MemTotal_bytes %.0f\n", float64(vm.Total))
		fmt.Fprintf(b, "node_memory_MemFree_bytes %.0f\n", float64(vm.Free))
		fmt.Fprintf(b, "node_memory_MemAvailable_bytes %.0f\n", float64(vm.Available))
		fmt.Fprintf(b, "node_memory_Buffers_bytes %.0f\n", float64(vm.Buffers))
		fmt.Fprintf(b, "node_memory_Cached_bytes %.0f\n", float64(vm.Cached))
	}

	if sw, err := mem.SwapMemory(); err == nil {
		fmt.Fprintf(b, "node_memory_SwapTotal_bytes %.0f\n", float64(sw.Total))
		fmt.Fprintf(b, "node_memory_SwapFree_bytes %.0f\n", float64(sw.Free))
	}
}

// writeFilesystemMetrics emits node_filesystem_* (size/avail/free + inodes).
func writeFilesystemMetrics(b *strings.Builder) {
	parts, err := disk.Partitions(false)
	if err != nil {
		return
	}
	b.WriteString("# HELP node_filesystem_size_bytes Filesystem size in bytes.\n")
	b.WriteString("# TYPE node_filesystem_size_bytes gauge\n")
	for _, p := range parts {
		u, err := disk.Usage(p.Mountpoint)
		if err != nil {
			continue
		}
		mp := escapeLabel(p.Mountpoint)
		fs := escapeLabel(p.Fstype)
		fmt.Fprintf(b, "node_filesystem_size_bytes{mountpoint=\"%s\",fstype=\"%s\"} %.0f\n", mp, fs, float64(u.Total))
		fmt.Fprintf(b, "node_filesystem_avail_bytes{mountpoint=\"%s\",fstype=\"%s\"} %.0f\n", mp, fs, float64(u.Free))
		fmt.Fprintf(b, "node_filesystem_free_bytes{mountpoint=\"%s\",fstype=\"%s\"} %.0f\n", mp, fs, float64(u.Free))
		fmt.Fprintf(b, "node_filesystem_files{mountpoint=\"%s\",fstype=\"%s\"} %.0f\n", mp, fs, float64(u.InodesTotal))
		fmt.Fprintf(b, "node_filesystem_files_free{mountpoint=\"%s\",fstype=\"%s\"} %.0f\n", mp, fs, float64(u.InodesFree))
	}
}

// writeDiskIOMetrics emits node_disk_* (per-device IO counters, cumulative).
func writeDiskIOMetrics(b *strings.Builder) {
	stats, err := disk.IOCounters()
	if err != nil || len(stats) == 0 {
		return
	}
	b.WriteString("# HELP node_disk_read_bytes_total The total number of bytes read successfully.\n")
	b.WriteString("# TYPE node_disk_read_bytes_total counter\n")
	for name, s := range stats {
		dev := escapeLabel(name)
		fmt.Fprintf(b, "node_disk_reads_completed_total{device=\"%s\"} %.0f\n", dev, float64(s.ReadCount))
		fmt.Fprintf(b, "node_disk_writes_completed_total{device=\"%s\"} %.0f\n", dev, float64(s.WriteCount))
		fmt.Fprintf(b, "node_disk_read_bytes_total{device=\"%s\"} %.0f\n", dev, float64(s.ReadBytes))
		fmt.Fprintf(b, "node_disk_written_bytes_total{device=\"%s\"} %.0f\n", dev, float64(s.WriteBytes))
		// gopsutil reports milliseconds; node_exporter reports seconds.
		fmt.Fprintf(b, "node_disk_read_time_seconds_total{device=\"%s\"} %.3f\n", dev, float64(s.ReadTime)/1000)
		fmt.Fprintf(b, "node_disk_write_time_seconds_total{device=\"%s\"} %.3f\n", dev, float64(s.WriteTime)/1000)
		fmt.Fprintf(b, "node_disk_io_time_seconds_total{device=\"%s\"} %.3f\n", dev, float64(s.IoTime)/1000)
		fmt.Fprintf(b, "node_disk_io_time_weighted_seconds_total{device=\"%s\"} %.3f\n", dev, float64(s.WeightedIO)/1000)
	}
}

// writeNetworkMetrics emits node_network_* (per-device traffic counters).
func writeNetworkMetrics(b *strings.Builder) {
	counters, err := gnet.IOCounters(true)
	if err != nil {
		return
	}
	b.WriteString("# HELP node_network_receive_bytes_total Network device statistic receive_bytes.\n")
	b.WriteString("# TYPE node_network_receive_bytes_total counter\n")
	for _, c := range counters {
		dev := escapeLabel(c.Name)
		fmt.Fprintf(b, "node_network_receive_bytes_total{device=\"%s\"} %.0f\n", dev, float64(c.BytesRecv))
		fmt.Fprintf(b, "node_network_transmit_bytes_total{device=\"%s\"} %.0f\n", dev, float64(c.BytesSent))
		fmt.Fprintf(b, "node_network_receive_packets_total{device=\"%s\"} %.0f\n", dev, float64(c.PacketsRecv))
		fmt.Fprintf(b, "node_network_transmit_packets_total{device=\"%s\"} %.0f\n", dev, float64(c.PacketsSent))
		fmt.Fprintf(b, "node_network_receive_errs_total{device=\"%s\"} %.0f\n", dev, float64(c.Errin))
		fmt.Fprintf(b, "node_network_transmit_errs_total{device=\"%s\"} %.0f\n", dev, float64(c.Errout))
		fmt.Fprintf(b, "node_network_receive_drop_total{device=\"%s\"} %.0f\n", dev, float64(c.Dropin))
		fmt.Fprintf(b, "node_network_transmit_drop_total{device=\"%s\"} %.0f\n", dev, float64(c.Dropout))
	}
}

// writeLoadMetrics emits node_load1/5/15.
func writeLoadMetrics(b *strings.Builder) {
	l, err := load.Avg()
	if err != nil {
		return
	}
	b.WriteString("# HELP node_load1 1m load average.\n")
	b.WriteString("# TYPE node_load1 gauge\n")
	fmt.Fprintf(b, "node_load1 %.2f\nnode_load5 %.2f\nnode_load15 %.2f\n", l.Load1, l.Load5, l.Load15)
}

// writeProcMetrics emits node_forks_total / node_procs_running / node_procs_blocked / node_context_switches_total
// plus node_procs_total (total number of processes, from /proc).
func writeProcMetrics(b *strings.Builder) {
	m, err := load.Misc()
	if err == nil {
		b.WriteString("# HELP node_forks_total Total number of forks.\n")
		b.WriteString("# TYPE node_forks_total counter\n")
		fmt.Fprintf(b, "node_forks_total %d\n", m.ProcsTotal)
		fmt.Fprintf(b, "node_procs_running %d\n", m.ProcsRunning)
		fmt.Fprintf(b, "node_procs_blocked %d\n", m.ProcsBlocked)
		fmt.Fprintf(b, "node_context_switches_total %d\n", m.Ctxt)
	}

	// node_procs_total: 系统总进程数（/proc 下 PID 数量），用于监控图表「进程数」。
	// node_procs_running 只是运行态(runnable)进程数，通常为个位数，不能代表总进程数。
	b.WriteString("# HELP node_procs_total Total number of processes.\n")
	b.WriteString("# TYPE node_procs_total gauge\n")
	if pids, err := process.Pids(); err == nil {
		fmt.Fprintf(b, "node_procs_total %d\n", len(pids))
	} else {
		fmt.Fprintf(b, "node_procs_total 0\n")
	}
}

// writeNetstatMetrics emits node_netstat_Tcp_* (TCP connection counters).
func writeNetstatMetrics(b *strings.Builder) {
	protos, err := gnet.ProtoCounters([]string{"tcp"})
	if err != nil || len(protos) == 0 {
		return
	}
	stats := protos[0].Stats
	// Only the commonly used fields; keep aligned with node_exporter netstat collector.
	fields := map[string]string{
		"CurrEstab":    "node_netstat_Tcp_CurrEstab",
		"ActiveOpens":  "node_netstat_Tcp_ActiveOpens",
		"PassiveOpens": "node_netstat_Tcp_PassiveOpens",
		"EstabResets":  "node_netstat_Tcp_EstabResets",
		"InSegs":       "node_netstat_Tcp_InSegs",
		"OutSegs":      "node_netstat_Tcp_OutSegs",
		"RetransSegs":  "node_netstat_Tcp_RetransSegs",
		"InErrs":       "node_netstat_Tcp_InErrs",
		"OutRsts":      "node_netstat_Tcp_OutRsts",
	}
	b.WriteString("# HELP node_netstat_Tcp_CurrEstab Statistic CurrEstab.\n")
	b.WriteString("# TYPE node_netstat_Tcp_CurrEstab gauge\n")
	for k, metric := range fields {
		if v, ok := stats[k]; ok {
			fmt.Fprintf(b, "%s %d\n", metric, v)
		}
	}
}

// writeThermalMetrics emits node_thermal_zone_temp (per-zone temperature in Celsius).
func writeThermalMetrics(b *strings.Builder) {
	temps, err := sensors.SensorsTemperatures()
	if err != nil {
		return
	}
	b.WriteString("# HELP node_thermal_zone_temp Zone temperature in Celsius.\n")
	b.WriteString("# TYPE node_thermal_zone_temp gauge\n")
	for _, t := range temps {
		fmt.Fprintf(b, "node_thermal_zone_temp{zone=\"%s\"} %.2f\n", escapeLabel(t.SensorKey), t.Temperature)
	}
}

// writeHostMetrics emits node_uname_info, node_boot_time_seconds and node_time_seconds.
func writeHostMetrics(b *strings.Builder) {
	if info, err := host.Info(); err == nil {
		b.WriteString("# HELP node_uname_info Labeled system information.\n")
		b.WriteString("# TYPE node_uname_info gauge\n")
		fmt.Fprintf(b, "node_uname_info{nodename=\"%s\",sysname=\"%s\",release=\"%s\",machine=\"%s\"} 1\n",
			escapeLabel(info.Hostname), escapeLabel(info.OS), escapeLabel(info.KernelVersion), escapeLabel(info.KernelArch))
		fmt.Fprintf(b, "node_boot_time_seconds %.0f\n", float64(info.BootTime))
	}
	fmt.Fprintf(b, "node_time_seconds %.0f\n", float64(time.Now().Unix()))
}

// escapeLabel escapes special characters in a Prometheus label value.
func escapeLabel(s string) string {
	r := strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n")
	return r.Replace(s)
}
