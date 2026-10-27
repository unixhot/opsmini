package sentinel

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// Processes that a web server user must never spawn. If one appears under
// www-data / nginx / apache it is either a webshell or a reverse shell.
var shellNames = map[string]bool{
	"sh": true, "bash": true, "dash": true, "zsh": true, "ksh": true,
	"curl": true, "wget": true, "nc": true, "ncat": true, "netcat": true,
	"socat": true, "python": true, "python3": true, "perl": true, "ruby": true,
	"php": true, "lua": true, "telnet": true,
}

var minerNames = []string{"minerd", "kdevtmpfsi", "xmrig", "kinsing", "sysupdate", "networkservice"}

var reverseShellMarkers = []string{"/dev/tcp/", "bash -i", "sh -i", "nc -e", "ncat -e", "socat", "exec 5<>", "0<&196"}

// procWatcher samples /proc looking for process trees that should not exist.
//
// Polling (not eBPF) means very short lived processes can be missed; that is a
// documented limitation. Everything it does see is evaluated on parent chain and
// owner, which keeps the false positive rate low.
type procWatcher struct {
	cfg      Config
	webUIDs  map[int]bool
	mu       sync.Mutex
	reported map[string]time.Time
}

func newProcWatcher(cfg Config) *procWatcher {
	uids := map[int]bool{}
	for _, name := range []string{"www-data", "nginx", "apache", "httpd", "lighttpd", "tomcat"} {
		if uid := userUID(name); uid > 0 {
			uids[uid] = true
		}
	}
	return &procWatcher{cfg: cfg, webUIDs: uids, reported: map[string]time.Time{}}
}

func (w *procWatcher) Name() string { return "procwatch" }

func (w *procWatcher) Run(ctx context.Context, out func(Signal)) {
	interval := w.cfg.PollInterval
	if interval <= 0 {
		interval = 2 * time.Second
	}
	t := time.NewTicker(interval)
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

func (w *procWatcher) scan(out func(Signal)) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
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

	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		comm := procComm(pid)
		if comm == "" {
			continue
		}
		lower := strings.ToLower(comm)

		// 1. Known miner / malware names.
		for _, m := range minerNames {
			if strings.Contains(lower, m) {
				w.emit(out, "proc.miner", pid, comm, model.SeverityCritical,
					"发现疑似挖矿进程："+comm+" (pid "+itoa(int64(pid))+")",
					map[string]any{"pid": pid, "comm": comm, "cmdline": procCmdline(pid)})
				break
			}
		}

		cmdline := procCmdline(pid)
		lowerCmd := strings.ToLower(cmdline)

		// 2. Reverse shell markers in the command line.
		for _, marker := range reverseShellMarkers {
			if strings.Contains(lowerCmd, marker) {
				w.emit(out, "proc.reverse_shell", pid, comm, model.SeverityCritical,
					"疑似反弹 Shell："+truncate(cmdline, 120),
					map[string]any{"pid": pid, "comm": comm, "cmdline": cmdline,
						"ppid": procPPID(pid), "parent": procCmdline(procPPID(pid))})
				break
			}
		}

		// 3. Web server user spawning an interpreter or downloader.
		if shellNames[lower] {
			uid := procUID(pid)
			if w.webUIDs[uid] {
				ppid := procPPID(pid)
				w.emit(out, "proc.web_shell_spawn", pid, comm, model.SeverityCritical,
					"Web 用户派生出危险子进程："+comm+" (pid "+itoa(int64(pid))+")",
					map[string]any{"pid": pid, "comm": comm, "cmdline": cmdline, "uid": uid,
						"ppid": ppid, "parent": procCmdline(ppid)})
			}
		}
	}
}

// emit publishes at most one signal per fingerprint+pid per 30 minutes.
func (w *procWatcher) emit(out func(Signal), fp string, pid int, comm, sev, msg string, evidence map[string]any) {
	key := fp + ":" + strconv.Itoa(pid)
	if _, ok := w.reported[key]; ok {
		return
	}
	w.reported[key] = time.Now()
	subj := comm + ":" + strconv.Itoa(pid)
	out(newSignal(model.SignalKindProc, "procwatch", subj, "proc.anomaly", fp, sev, msg).
		withTags("scenario:proc_anomaly").
		withEvidence(evidence))
}
