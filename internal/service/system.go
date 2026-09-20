package service

import (
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/load"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// SystemService provides real-time collection of host system information.
type SystemService struct {
	netMu       sync.Mutex
	netDayKey   string // today's baseline date "2006-01-02"
	netBaseRec  uint64 // baseline cumulative received bytes at 0:00 today
	netBaseSent uint64 // baseline cumulative sent bytes at 0:00 today
}

// NewSystemService creates a SystemService.
func NewSystemService() *SystemService { return &SystemService{} }

// HostInfo is host information.
type HostInfo struct {
	Hostname    string   `json:"hostname"`
	OS          string   `json:"os"`
	Platform    string   `json:"platform"`
	PlatformVer string   `json:"platform_version"`
	KernelVer   string   `json:"kernel_version"`
	KernelArch  string   `json:"kernel_arch"`
	CPUModel    string   `json:"cpu_model"`
	CPUCores    int      `json:"cpu_cores"`
	Uptime      uint64   `json:"uptime"` // seconds
	IPs         []string `json:"ips,omitempty"`
	DNS         []string `json:"dns,omitempty"`
}

// MonitorSummary is the aggregated monitor page overview (host info + performance + statistic counts).
type MonitorSummary struct {
	// basic data
	Hostname    string   `json:"hostname"`
	IPs         []string `json:"ips"`
	OS          string   `json:"os"`
	Platform    string   `json:"platform"`
	PlatformVer string   `json:"platform_version"`
	KernelVer   string   `json:"kernel_version"`
	KernelArch  string   `json:"kernel_arch"`
	CPUModel    string   `json:"cpu_model"`
	CPUCores    int      `json:"cpu_cores"`
	DNS         []string `json:"dns"`
	Uptime      uint64   `json:"uptime"`
	// performance data (by item)
	CPUPercent  float64 `json:"cpu_percent"`
	MemPercent  float64 `json:"mem_percent"`
	MemTotal    uint64  `json:"mem_total"`
	MemUsed     uint64  `json:"mem_used"`
	Load1       float64 `json:"load1"`
	Load5       float64 `json:"load5"`
	Load15      float64 `json:"load15"`
	DiskPercent float64 `json:"disk_percent"`
	DiskTotal   uint64  `json:"disk_total"`
	DiskUsed    uint64  `json:"disk_used"`
	// statistic counts
	ProcessCount   int `json:"process_count"`
	UserCount      int `json:"user_count"`
	LoginUserCount int `json:"login_user_count"`
	PortCount      int `json:"port_count"`
	TcpPortCount   int `json:"tcp_port_count"`
	UdpPortCount   int `json:"udp_port_count"`
	ZombieCount    int `json:"zombie_count"`
}

// Overview is the dashboard overview.
type Overview struct {
	Hostname     string   `json:"hostname"`
	OS           string   `json:"os"`
	Platform     string   `json:"platform"`         // system type (e.g. ubuntu/centos)
	PlatformVer  string   `json:"platform_version"` // distro version (e.g. 22.04)
	KernelVer    string   `json:"kernel_version"`   // kernel version (e.g. 6.5.0-27)
	KernelArch   string   `json:"kernel_arch"`
	IPs          []string `json:"ips"`              // host IPv4 address list
	BootTime     int64    `json:"boot_time"`        // boot time (unix seconds)
	CPUPercent   float64  `json:"cpu_percent"`
	CPUCores     int      `json:"cpu_cores"`
	MemPercent   float64  `json:"mem_percent"`
	MemTotal     uint64   `json:"mem_total"`
	MemUsed      uint64   `json:"mem_used"`
	Load1        float64  `json:"load1"`
	Load5        float64  `json:"load5"`
	Load15       float64  `json:"load15"`
	Uptime       uint64   `json:"uptime"`
	DiskPercent  float64  `json:"disk_percent"`  // root partition
	DiskTotal    uint64   `json:"disk_total"`    // root partition total capacity (bytes)
	DiskUsed     uint64   `json:"disk_used"`     // root partition used (bytes)
	NetRecvRate  uint64   `json:"net_recv_rate"` // bytes/s
	NetSentRate  uint64   `json:"net_sent_rate"` // bytes/s
	NetRecvTotal uint64   `json:"net_recv_total"` // today's cumulative received (bytes)
	NetSentTotal uint64   `json:"net_sent_total"` // today's cumulative sent (bytes)
}

// ProcessInfo is process information.
type ProcessInfo struct {
	PID        int32   `json:"pid"`
	Name       string  `json:"name"`
	User       string  `json:"user"`
	MemPercent float32 `json:"mem_percent"`
	Cmdline    string  `json:"cmdline"`
	Kind       string  `json:"kind"` // system / app
}

// PortInfo is a listening port.
type PortInfo struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     uint32 `json:"port"`
	PID      int32  `json:"pid"`
	Name     string `json:"name"`    // process name (may be empty)
	Cmdline  string `json:"cmdline"` // process command line (shown by the frontend when name is unavailable)
}

// DiskInfo is a disk partition.
type DiskInfo struct {
	Device      string  `json:"device"`
	Mountpoint  string  `json:"mountpoint"`
	Fstype      string  `json:"fstype"`
	Total       uint64  `json:"total"`
	Used        uint64  `json:"used"`
	Free        uint64  `json:"free"`
	UsedPercent float64 `json:"used_percent"`
}

// NetInterface is a network interface.
type NetInterface struct {
	Name      string   `json:"name"`
	MTU       int      `json:"mtu"`
	Type      string   `json:"type"` // physical / virtual / loopback
	Addrs     []string `json:"addrs,omitempty"`
	BytesRecv uint64   `json:"bytes_recv"`
	BytesSent uint64   `json:"bytes_sent"`
}

// RouteEntry is a kernel IP routing table entry (from /proc/net/route).
type RouteEntry struct {
	Destination string `json:"destination"` // destination network (0.0.0.0 = default)
	Gateway     string `json:"gateway"`     // gateway (0.0.0.0 = none)
	Genmask     string `json:"genmask"`     // subnet mask
	Flags       int    `json:"flags"`       // route flags (UP=1, Gateway=2, Host=4)
	Iface       string `json:"iface"`       // network interface
	Metric      int    `json:"metric"`      // route metric
}

// HostInfo returns static host information.
func (s *SystemService) HostInfo() (*HostInfo, error) {
	info, err := host.Info()
	if err != nil {
		return nil, err
	}
	cores, _ := cpu.Counts(true)
	model := ""
	if cis, err := cpu.Info(); err == nil && len(cis) > 0 {
		model = strings.TrimSpace(cis[0].ModelName)
	}
	return &HostInfo{
		Hostname:    info.Hostname,
		OS:          info.OS,
		Platform:    info.Platform,
		PlatformVer: info.PlatformVersion,
		KernelVer:   info.KernelVersion,
		KernelArch:  info.KernelArch,
		CPUModel:    model,
		CPUCores:    cores,
		Uptime:      info.Uptime,
		IPs:         s.ipAddresses(),
		DNS:         s.dnsServers(),
	}, nil
}

// ipAddresses returns the list of non-loopback IPv4 addresses.
func (s *SystemService) ipAddresses() []string {
	ifaces, err := gnet.Interfaces()
	if err != nil {
		return nil
	}
	var ips []string
	for _, iface := range ifaces {
		for _, a := range iface.Addrs {
			ip := a.Addr
			if ip == "" || strings.HasPrefix(ip, "127.") || strings.HasPrefix(ip, "::1") {
				continue
			}
			if i := strings.IndexByte(ip, '/'); i >= 0 {
				ip = ip[:i]
			}
			if strings.Contains(ip, ":") {
				continue // keep IPv4 only
			}
			ips = append(ips, ip)
		}
	}
	return ips
}

// dnsServers reads DNS servers from /etc/resolv.conf.
func (s *SystemService) dnsServers() []string {
	data, err := os.ReadFile("/etc/resolv.conf")
	if err != nil {
		return nil
	}
	var dns []string
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 && f[0] == "nameserver" {
			dns = append(dns, f[1])
		}
	}
	return dns
}

// MonitorSummary returns the aggregated monitor page overview (basic data + performance + statistic counts).
func (s *SystemService) MonitorSummary() (*MonitorSummary, error) {
	hi, err := s.HostInfo()
	if err != nil {
		return nil, err
	}
	// performance: CPU sampling 300ms
	window := 300 * time.Millisecond
	cpuPerc, _ := cpu.Percent(window, false)
	var cpuVal float64
	if len(cpuPerc) > 0 {
		cpuVal = cpuPerc[0]
	}
	vm, _ := mem.VirtualMemory()
	lavg, _ := load.Avg()

	sum := &MonitorSummary{
		Hostname:    hi.Hostname,
		IPs:         hi.IPs,
		OS:          hi.OS,
		Platform:    hi.Platform,
		PlatformVer: hi.PlatformVer,
		KernelVer:   hi.KernelVer,
		KernelArch:  hi.KernelArch,
		CPUModel:    hi.CPUModel,
		CPUCores:    hi.CPUCores,
		DNS:         hi.DNS,
		Uptime:      hi.Uptime,
		CPUPercent:  cpuVal,
	}
	if vm != nil {
		sum.MemPercent = vm.UsedPercent
		sum.MemTotal = vm.Total
		sum.MemUsed = vm.Used
	}
	if lavg != nil {
		sum.Load1, sum.Load5, sum.Load15 = lavg.Load1, lavg.Load5, lavg.Load15
	}
	if du, err := disk.Usage("/"); err == nil {
		sum.DiskPercent = du.UsedPercent
		sum.DiskTotal = du.Total
		sum.DiskUsed = du.Used
	}
	// statistic counts
	if procs, err := process.Processes(); err == nil {
		sum.ProcessCount = len(procs)
		for _, p := range procs {
			if st, err := p.Status(); err == nil {
				for _, s := range st {
					if strings.ToLower(s) == "z" || strings.ToLower(s) == "zombie" {
						sum.ZombieCount++
						break
					}
				}
			}
		}
	}
	sum.UserCount, sum.LoginUserCount = countUsers()
	if ports, err := s.Ports(); err == nil {
		sum.PortCount = len(ports)
		for _, p := range ports {
			switch strings.ToLower(p.Protocol) {
			case "tcp":
				sum.TcpPortCount++
			case "udp":
				sum.UdpPortCount++
			}
		}
	}
	return sum, nil
}

// loginShellsOnce guards lazy loading of valid login shells from /etc/shells.
var (
	loginShellsOnce sync.Once
	loginShells     map[string]bool
)

// isLoginShell reports whether the shell is a valid login shell (i.e. listed in /etc/shells).
// System accounts like sync/shutdown/halt have pseudo-shells (/bin/sync, /sbin/shutdown, /sbin/halt)
// that are not real login shells, so they must NOT be considered loggable. Merely checking for
// "nologin"/"false" is insufficient and wrongly marks those accounts as loggable.
func isLoginShell(shell string) bool {
	if shell == "" {
		return false
	}
	loginShellsOnce.Do(func() {
		loginShells = map[string]bool{}
		data, err := os.ReadFile("/etc/shells")
		if err != nil {
			loginShells = nil // signal fallback
			return
		}
		for _, line := range strings.Split(string(data), "\n") {
			s := strings.TrimSpace(line)
			if s != "" && !strings.HasPrefix(s, "#") {
				loginShells[s] = true
			}
		}
	})
	if loginShells == nil {
		// Fallback when /etc/shells is unavailable: treat nologin/false as non-login,
		// and only allow a known set of common login shells.
		if strings.Contains(shell, "nologin") || strings.Contains(shell, "false") {
			return false
		}
		switch shell {
		case "/bin/bash", "/bin/sh", "/bin/zsh", "/bin/dash", "/bin/fish", "/bin/ksh", "/usr/bin/bash", "/usr/bin/zsh":
			return true
		}
		return false
	}
	return loginShells[shell]
}

// countUsers counts total users and loggable users from /etc/passwd.
func countUsers() (total, login int) {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return 0, 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		total++
		if isLoginShell(parts[6]) {
			login++
		}
	}
	return total, login
}

// Overview returns the dashboard overview (CPU/memory/load/disk/network rate).
// CPU and network rate need a short observation window, so the API has about 300ms latency.
func (s *SystemService) Overview() (*Overview, error) {
	hi, err := s.HostInfo()
	if err != nil {
		return nil, err
	}

	// network rate: observation-window sampling
	window := 300 * time.Millisecond
	io0, _ := gnet.IOCounters(false)
	var recv0, sent0 uint64
	if len(io0) > 0 {
		recv0, sent0 = io0[0].BytesRecv, io0[0].BytesSent
	}

	cpuPerc, _ := cpu.Percent(window, false)
	var cpuVal float64
	if len(cpuPerc) > 0 {
		cpuVal = cpuPerc[0]
	}

	io1, _ := gnet.IOCounters(false)
	var recvRate, sentRate uint64
	if len(io0) > 0 && len(io1) > 0 {
		elapsed := window.Seconds()
		if elapsed > 0 {
			recvRate = uint64(float64(io1[0].BytesRecv-recv0) / elapsed)
			sentRate = uint64(float64(io1[0].BytesSent-sent0) / elapsed)
		}
	}

	vm, _ := mem.VirtualMemory()
	lavg, _ := load.Avg()

	// today's cumulative traffic: baseline at 0:00 today, auto-reset across days.
	now := time.Now()
	dayKey := now.Format("2006-01-02")
	s.netMu.Lock()
	if s.netDayKey != dayKey {
		s.netDayKey = dayKey
		s.netBaseRec = recv0
		s.netBaseSent = sent0
	}
	recvTotal := recv0 - s.netBaseRec
	sentTotal := sent0 - s.netBaseSent
	s.netMu.Unlock()

	o := &Overview{
		Hostname:     hi.Hostname,
		OS:           hi.OS,
		Platform:     hi.Platform,
		PlatformVer:  hi.PlatformVer,
		KernelVer:    hi.KernelVer,
		KernelArch:   hi.KernelArch,
		IPs:          hi.IPs,
		CPUPercent:   cpuVal,
		CPUCores:     hi.CPUCores,
		Uptime:       hi.Uptime,
		NetRecvRate:  recvRate,
		NetSentRate:  sentRate,
		NetRecvTotal: recvTotal,
		NetSentTotal: sentTotal,
	}
	if bt, err := host.BootTime(); err == nil {
		o.BootTime = int64(bt)
	}
	if vm != nil {
		o.MemPercent = vm.UsedPercent
		o.MemTotal = vm.Total
		o.MemUsed = vm.Used
	}
	if lavg != nil {
		o.Load1, o.Load5, o.Load15 = lavg.Load1, lavg.Load5, lavg.Load15
	}

	// root partition usage
	if du, err := disk.Usage("/"); err == nil {
		o.DiskPercent = du.UsedPercent
		o.DiskTotal = du.Total
		o.DiskUsed = du.Used
	}
	return o, nil
}

// Processes returns the process list (descending by memory usage, at most max entries).
func (s *SystemService) Processes(max int) ([]ProcessInfo, error) {
	if max <= 0 {
		max = 100
	}
	procs, err := process.Processes()
	if err != nil {
		return nil, err
	}
	list := make([]ProcessInfo, 0, len(procs))
	for _, p := range procs {
		pi := ProcessInfo{PID: p.Pid}
		if name, err := p.Name(); err == nil {
			pi.Name = name
		}
		if user, err := p.Username(); err == nil {
			pi.User = user
		}
		if mp, err := p.MemoryPercent(); err == nil {
			pi.MemPercent = mp
		}
		if cl, err := p.Cmdline(); err == nil {
			pi.Cmdline = truncate(cl, 512)
		}
		// only Linux kernel-level processes (no executable file, e.g. kthreadd/kernel threads) count as system processes; the rest (including /usr/sbin/nginx, etc.) count as app processes
		if exe, err := p.Exe(); err == nil && exe != "" {
			pi.Kind = "app"
		} else {
			pi.Kind = "system"
		}
		list = append(list, pi)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].MemPercent > list[j].MemPercent })
	if len(list) > max {
		list = list[:max]
	}
	return list, nil
}

// Ports returns the list of listening TCP and UDP ports.
func (s *SystemService) Ports() ([]PortInfo, error) {
	out := []PortInfo{}
	seen := map[string]bool{}
	// pid -> [process name, command line], to avoid repeated NewProcess
	procCache := map[int32][2]string{}
	resolveProc := func(pid int32) (string, string) {
		if pid <= 0 {
			return "", ""
		}
		if v, ok := procCache[pid]; ok {
			return v[0], v[1]
		}
		name, cmdline := "", ""
		if pr, err := process.NewProcess(pid); err == nil {
			if n, err := pr.Name(); err == nil {
				name = n
			}
			if cl, err := pr.Cmdline(); err == nil {
				cmdline = truncate(cl, 256)
			}
		}
		procCache[pid] = [2]string{name, cmdline}
		return name, cmdline
	}
	// TCP listening ports
	if conns, err := gnet.Connections("tcp"); err == nil {
		for _, c := range conns {
			if c.Status != "LISTEN" {
				continue
			}
			key := "tcp:" + itoa(uint64(c.Laddr.Port))
			if seen[key] {
				continue
			}
			seen[key] = true
			name, cmdline := resolveProc(c.Pid)
			out = append(out, PortInfo{
				Protocol: "tcp",
				Address:  c.Laddr.IP,
				Port:     c.Laddr.Port,
				PID:      c.Pid,
				Name:     name,
				Cmdline:  cmdline,
			})
		}
	}
	// UDP ports (UDP has no LISTEN state, take all local sockets and deduplicate)
	if conns, err := gnet.Connections("udp"); err == nil {
		for _, c := range conns {
			key := "udp:" + itoa(uint64(c.Laddr.Port))
			if seen[key] {
				continue
			}
			seen[key] = true
			name, cmdline := resolveProc(c.Pid)
			out = append(out, PortInfo{
				Protocol: "udp",
				Address:  c.Laddr.IP,
				Port:     c.Laddr.Port,
				PID:      c.Pid,
				Name:     name,
				Cmdline:  cmdline,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Protocol != out[j].Protocol {
			return out[i].Protocol < out[j].Protocol
		}
		return out[i].Port < out[j].Port
	})
	return out, nil
}

// Disks returns disk partitions and usage.
func (s *SystemService) Disks() ([]DiskInfo, error) {
	parts, err := disk.Partitions(false)
	if err != nil {
		return nil, err
	}
	var out []DiskInfo
	for _, p := range parts {
		// 过滤 squashfs（snap 只读 loop 挂载，df 默认通过 x-gdu.hide 隐藏它们），
		// 避免面板显示一堆 100% 使用率的 /snap 分区，与 df -h 展示不一致。
		if p.Fstype == "squashfs" {
			continue
		}
		di := DiskInfo{
			Device:     p.Device,
			Mountpoint: p.Mountpoint,
			Fstype:     p.Fstype,
		}
		if u, err := disk.Usage(p.Mountpoint); err == nil {
			di.Total, di.Used, di.Free, di.UsedPercent = u.Total, u.Used, u.Free, u.UsedPercent
		}
		out = append(out, di)
	}
	return out, nil
}

// Networks returns network interfaces and cumulative traffic.
func (s *SystemService) Networks() ([]NetInterface, error) {
	ifaces, err := gnet.Interfaces()
	if err != nil {
		return nil, err
	}
	counters, _ := gnet.IOCounters(true)
	counterMap := map[string]gnet.IOCountersStat{}
	for _, c := range counters {
		counterMap[c.Name] = c
	}

	var out []NetInterface
	for _, iface := range ifaces {
		ni := NetInterface{Name: iface.Name, MTU: iface.MTU, Type: classifyInterface(iface.Name, iface.Flags)}
		for _, a := range iface.Addrs {
			ni.Addrs = append(ni.Addrs, a.Addr)
		}
		if c, ok := counterMap[iface.Name]; ok {
			ni.BytesRecv, ni.BytesSent = c.BytesRecv, c.BytesSent
		}
		out = append(out, ni)
	}
	return out, nil
}

// Routes returns the kernel IP routing table (Linux reads /proc/net/route).
func (s *SystemService) Routes() ([]RouteEntry, error) {
	data, err := os.ReadFile("/proc/net/route")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	var out []RouteEntry
	for i, line := range lines {
		if i == 0 || strings.TrimSpace(line) == "" {
			continue // skip header and blank lines
		}
		fields := strings.Fields(line)
		if len(fields) < 8 {
			continue
		}
		flags, _ := strconv.ParseInt(fields[3], 16, 64)
		metric, _ := strconv.Atoi(fields[6])
		out = append(out, RouteEntry{
			Destination: hexToIPv4(fields[1]),
			Gateway:     hexToIPv4(fields[2]),
			Genmask:     hexToIPv4(fields[7]),
			Flags:       int(flags),
			Iface:       fields[0],
			Metric:      metric,
		})
	}
	return out, nil
}

// hexToIPv4 converts a little-endian 32-bit hex string from /proc/net/route to dotted-quad IP.
func hexToIPv4(hexStr string) string {
	v, err := strconv.ParseUint(hexStr, 16, 32)
	if err != nil {
		return hexStr
	}
	return fmt.Sprintf("%d.%d.%d.%d", byte(v), byte(v>>8), byte(v>>16), byte(v>>24))
}

// NetRate returns the current aggregate network receive/send rate in bytes per second
// using a short observation window (used by alert rules for "network bandwidth in/out").
func (s *SystemService) NetRate() (recvRate, sentRate uint64) {
	io0, _ := gnet.IOCounters(false)
	if len(io0) == 0 {
		return 0, 0
	}
	r0, s0 := io0[0].BytesRecv, io0[0].BytesSent
	time.Sleep(300 * time.Millisecond)
	io1, _ := gnet.IOCounters(false)
	if len(io1) == 0 {
		return 0, 0
	}
	const window = 0.3
	return uint64(float64(io1[0].BytesRecv-r0) / window), uint64(float64(io1[0].BytesSent-s0) / window)
}

// itoa converts a uint64 to its decimal string representation without allocation (used in port keys).
func itoa(v uint64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

// truncate truncates a string by bytes, appending "…" when it exceeds the limit.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// classifyInterface determines the interface type (loopback / virtual / physical) from its name and flags.
func classifyInterface(name string, flags []string) string {
	lower := strings.ToLower(name)
	// loopback interface
	if lower == "lo" {
		return "loopback"
	}
	for _, f := range flags {
		if strings.EqualFold(f, "loopback") {
			return "loopback"
		}
	}
	// common virtual interface prefixes: docker0, br-, veth, virbr, vnet, tun, tap, etc.
	for _, p := range []string{"docker", "br-", "veth", "virbr", "vnet", "tun", "tap"} {
		if strings.HasPrefix(lower, p) {
			return "virtual"
		}
	}
	return "physical"
}

// KillProcess sends a signal to a process (SIGTERM/SIGKILL, etc.).
func (s *SystemService) KillProcess(pid int32, sig syscall.Signal) error {
	p, err := process.NewProcess(pid)
	if err != nil {
		return err
	}
	return p.SendSignal(sig)
}

// UserAccount is a Linux system user.
type UserAccount struct {
	Username  string `json:"username"`
	UID       string `json:"uid"`
	GID       string `json:"gid"`
	Home      string `json:"home"`
	Shell     string `json:"shell"`
	Loginable bool   `json:"loginable"` // shell without nologin/false is considered loggable
}

// GroupAccount is a Linux user group.
type GroupAccount struct {
	Groupname string   `json:"groupname"`
	GID       string   `json:"gid"`
	Members   []string `json:"members"`
}

// FirewallStatus is the system firewall status.
type FirewallStatus struct {
	Enabled bool     `json:"enabled"`
	Backend string   `json:"backend"` // firewalld / ufw / iptables / none
	Rules   []string `json:"rules"`   // rule summaries (at most 20)
	Message string   `json:"message"`
}

// Users reads /etc/passwd and returns the system user list (distinguishing loggable/non-loggable).
func (s *SystemService) Users() []UserAccount {
	data, err := os.ReadFile("/etc/passwd")
	if err != nil {
		return nil
	}
	var out []UserAccount
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 7 {
			continue
		}
		shell := parts[6]
		out = append(out, UserAccount{
			Username:  parts[0],
			UID:       parts[2],
			GID:       parts[3],
			Home:      parts[5],
			Shell:     shell,
			Loginable: isLoginShell(shell),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UID < out[j].UID })
	return out
}

// Groups reads /etc/group and returns the user group list.
func (s *SystemService) Groups() []GroupAccount {
	data, err := os.ReadFile("/etc/group")
	if err != nil {
		return nil
	}
	var out []GroupAccount
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 4 {
			continue
		}
		var members []string
		if parts[3] != "" {
			members = strings.Split(parts[3], ",")
		}
		out = append(out, GroupAccount{Groupname: parts[0], GID: parts[2], Members: members})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].GID < out[j].GID })
	return out
}

// FirewallStatus detects the system firewall (firewalld / ufw / iptables), with a hint when none is enabled.
func (s *SystemService) FirewallStatus() *FirewallStatus {
	// firewalld (RHEL/CentOS/Alibaba Linux, etc.)
	if out, err := exec.Command("firewall-cmd", "--state").Output(); err == nil {
		if strings.TrimSpace(string(out)) == "running" {
			return &FirewallStatus{Enabled: true, Backend: "firewalld", Message: "firewalld 正在运行"}
		}
	}
	// ufw (Ubuntu/Debian)
	if out, err := exec.Command("ufw", "status").Output(); err == nil {
		s := string(out)
		if strings.Contains(s, "Status: active") {
			return &FirewallStatus{Enabled: true, Backend: "ufw", Message: "ufw 正在运行"}
		}
		// ufw 已安装但未启用（Status: inactive），仍识别为 ufw 后端，允许面板内启用
		if strings.Contains(s, "Status: inactive") {
			return &FirewallStatus{Enabled: false, Backend: "ufw", Message: "ufw 已安装但未启用"}
		}
	}
	// iptables: any rules mean enabled
	if out, err := exec.Command("iptables", "-S").Output(); err == nil {
		rules := strings.Split(strings.TrimSpace(string(out)), "\n")
		if len(rules) > 0 && strings.TrimSpace(rules[0]) != "" && rules[0] != "-P INPUT ACCEPT" {
			if len(rules) > 20 {
				rules = rules[:20]
			}
			return &FirewallStatus{Enabled: true, Backend: "iptables", Rules: rules, Message: "iptables 规则已配置"}
		}
	}
	return &FirewallStatus{Enabled: false, Backend: "none", Message: "未检测到系统防火墙（firewalld/ufw/iptables 均未启用）"}
}
