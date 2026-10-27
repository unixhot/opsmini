package sentinel

import (
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// Ports that are normal for a web worker to open. Anything else from a web user
// is worth a look.
var benignRemotePorts = map[int]bool{
	80: true, 443: true, 8080: true, 8443: true,
	3306: true, 5432: true, 6379: true, 27017: true,
	9200: true, 11211: true, 25: true, 587: true,
}

// Ports that almost only appear in hands-on-keyboard intrusions.
var suspiciousPorts = map[int]bool{
	4444: true, 1337: true, 31337: true, 6667: true, 5555: true, 9999: true, 12345: true,
}

// netWatcher maps established sockets back to the owning process and flags
// unusual egress, especially from web server users.
type netWatcher struct {
	cfg      Config
	webUIDs  map[int]bool
	mu       sync.Mutex
	reported map[string]time.Time
}

func newNetWatcher(cfg Config) *netWatcher {
	uids := map[int]bool{}
	for _, name := range []string{"www-data", "nginx", "apache", "httpd", "lighttpd", "tomcat"} {
		if uid := userUID(name); uid > 0 {
			uids[uid] = true
		}
	}
	return &netWatcher{cfg: cfg, webUIDs: uids, reported: map[string]time.Time{}}
}

func (w *netWatcher) Name() string { return "netwatch" }

func (w *netWatcher) Run(ctx context.Context, out func(Signal)) {
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.scan(out)
		}
	}
}

type remoteEP struct {
	ip   string
	port int
}

func (w *netWatcher) scan(out func(Signal)) {
	endpoints := map[string]remoteEP{} // socket inode -> remote endpoint
	for _, f := range []string{"/proc/net/tcp", "/proc/net/tcp6"} {
		collectEstablished(f, endpoints)
	}
	if len(endpoints) == 0 {
		return
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	now := time.Now()
	for k, t := range w.reported {
		if now.Sub(t) > 30*time.Minute {
			delete(w.reported, k)
		}
	}

	// Map socket inodes to owning pids.
	owners := socketOwners()
	for inode, ep := range endpoints {
		pid, ok := owners[inode]
		if !ok {
			continue
		}
		uid := procUID(pid)
		comm := procComm(pid)

		if suspiciousPorts[ep.port] {
			key := "net.suspicious_port:" + ep.ip + ":" + strconv.Itoa(ep.port)
			if _, seen := w.reported[key]; !seen {
				w.reported[key] = now
				out(newSignal(model.SignalKindNet, "netwatch", ep.ip+":"+strconv.Itoa(ep.port),
					"net.anomaly", "net.suspicious_port", model.SeverityHigh,
					"外联到可疑端口 "+ep.ip+":"+strconv.Itoa(ep.port)+"（"+comm+"）").
					withTags("mitre:T1571", "scenario:egress").
					withEvidence(map[string]any{"remote": ep.ip, "port": ep.port,
						"pid": pid, "comm": comm, "cmdline": procCmdline(pid)}))
			}
			continue
		}

		if w.webUIDs[uid] && !benignRemotePorts[ep.port] {
			key := "net.web_user_outbound:" + ep.ip + ":" + strconv.Itoa(ep.port)
			if _, seen := w.reported[key]; !seen {
				w.reported[key] = now
				out(newSignal(model.SignalKindNet, "netwatch", ep.ip+":"+strconv.Itoa(ep.port),
					"net.anomaly", "net.web_user_outbound", model.SeverityMedium,
					"Web 用户发起非常规外联 "+ep.ip+":"+strconv.Itoa(ep.port)+"（"+comm+"）").
					withTags("scenario:egress").
					withEvidence(map[string]any{"remote": ep.ip, "port": ep.port,
						"pid": pid, "comm": comm, "uid": uid}))
			}
		}
	}
}

// collectEstablished parses /proc/net/tcp* keeping ESTABLISHED sockets only.
func collectEstablished(path string, out map[string]remoteEP) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(b), "\n")
	for i, ln := range lines {
		if i == 0 || strings.TrimSpace(ln) == "" {
			continue
		}
		f := strings.Fields(ln)
		if len(f) < 10 {
			continue
		}
		if f[3] != "01" { // 01 = TCP_ESTABLISHED
			continue
		}
		ip, port, ok := parseHexAddr(f[2])
		if !ok {
			continue
		}
		out[f[9]] = remoteEP{ip: ip, port: port}
	}
}

// parseHexAddr decodes "0100007F:1F90" style addresses (little endian).
func parseHexAddr(s string) (string, int, bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return "", 0, false
	}
	port64, err := strconv.ParseInt(parts[1], 16, 32)
	if err != nil {
		return "", 0, false
	}
	raw, err := hex.DecodeString(parts[0])
	if err != nil || len(raw) < 4 {
		return "", 0, false
	}
	// IPv4 in /proc/net/tcp is a little-endian uint32.
	ip := strconv.Itoa(int(raw[3])) + "." + strconv.Itoa(int(raw[2])) + "." +
		strconv.Itoa(int(raw[1])) + "." + strconv.Itoa(int(raw[0]))
	if len(raw) == 16 {
		// IPv6: compress into the familiar form, drop the v4-mapped prefix.
		if isV4Mapped(raw) {
			ip = strconv.Itoa(int(raw[12])) + "." + strconv.Itoa(int(raw[13])) + "." +
				strconv.Itoa(int(raw[14])) + "." + strconv.Itoa(int(raw[15]))
		} else {
			ip = "[" + hex.EncodeToString(raw) + "]"
		}
	}
	return ip, int(port64), true
}

func isV4Mapped(raw []byte) bool {
	for i := 0; i < 10; i++ {
		if raw[i] != 0 {
			return false
		}
	}
	return raw[10] == 0xff && raw[11] == 0xff
}

// socketOwners maps socket inode -> pid by scanning fd tables (bounded).
func socketOwners() map[string]int {
	out := map[string]int{}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return out
	}
	scanned := 0
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		fdDir := filepath.Join("/proc", e.Name(), "fd")
		fds, err := os.ReadDir(fdDir)
		if err != nil {
			continue
		}
		for _, fd := range fds {
			scanned++
			if scanned > 20000 {
				return out
			}
			target, err := os.Readlink(filepath.Join(fdDir, fd.Name()))
			if err != nil || !strings.HasPrefix(target, "socket:[") {
				continue
			}
			inode := strings.TrimSuffix(strings.TrimPrefix(target, "socket:["), "]")
			out[inode] = pid
		}
	}
	return out
}
