package logpipe

import (
	"net"
	"os"
	"runtime"
	"time"
)

// Event is one log line after parsing. A map keeps it close to what filebeat
// produces on the wire, so processors and outputs both work on the same shape.
type Event map[string]any

// ecsVersion mirrors the ECS version filebeat 8.x stamps on every document.
// Index templates match on it, so it must not be invented.
const ecsVersion = "8.11.0"

// hostMeta is resolved once: calling the OS for every line is wasteful and the
// answer never changes while the process runs.
type hostInfo struct{ name, os, arch, ip string }

var hostMeta = func() hostInfo {
	name, _ := os.Hostname()
	ip := ""
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
				ip = ipnet.IP.String()
				break
			}
		}
	}
	return hostInfo{name: name, os: runtime.GOOS, arch: runtime.GOARCH, ip: ip}
}()

// newEvent builds the base document for one line.
func newEvent(task Task, path string, offset int64, line string, agentType, agentVersion string) Event {
	ev := Event{
		"@timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"message":    line,
		"log": Event{
			"file":   Event{"path": path},
			"offset": offset,
		},
		"input": Event{"type": task.Input.Type, "id": task.ID},
		"event": Event{"dataset": task.Input.ID},
		"host": Event{
			"name": hostMeta.name, "os": Event{"type": hostMeta.os}, "arch": hostMeta.arch,
			"ip": hostMeta.ip,
		},
		"agent":  Event{"type": agentType, "version": agentVersion, "id": hostMeta.name},
		"ecs":    Event{"version": ecsVersion},
		"fields": Event{"task_id": task.ID, "service": task.Input.ID},
	}
	// task-declared fields: filebeat puts them under fields.* unless
	// fields_under_root is set, and users rely on that default.
	for k, v := range task.Input.Fields {
		if k == "fields_under_root" {
			continue
		}
		ev["fields"].(Event)[k] = v
	}
	return ev
}

// lowerFieldPath sets a nested key using dot notation (a.b.c), which is how
// most filebeat processors address fields.
func setPath(ev Event, path string, val any) {
	if path == "" {
		return
	}
	parts := splitPath(path)
	cur := map[string]any(ev)
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p] = val
			return
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			if e, ok2 := cur[p].(Event); ok2 {
				next = map[string]any(e)
				cur[p] = next
			} else {
				next = map[string]any{}
				cur[p] = next
			}
		}
		cur = next
	}
}

// getPath reads a nested key using dot notation.
func getPath(ev Event, path string) (any, bool) {
	parts := splitPath(path)
	var cur any = map[string]any(ev)
	for _, p := range parts {
		switch m := cur.(type) {
		case map[string]any:
			v, ok := m[p]
			if !ok {
				return nil, false
			}
			cur = v
		case Event:
			v, ok := m[p]
			if !ok {
				return nil, false
			}
			cur = v
		default:
			return nil, false
		}
	}
	return cur, true
}

// delPath removes a nested key.
func delPath(ev Event, path string) {
	parts := splitPath(path)
	if len(parts) == 0 {
		return
	}
	cur := map[string]any(ev)
	for i, p := range parts {
		if i == len(parts)-1 {
			delete(cur, p)
			return
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			return
		}
		cur = next
	}
}

func splitPath(p string) []string {
	var out []string
	start := 0
	for i := 0; i <= len(p); i++ {
		if i == len(p) || p[i] == '.' {
			if i > start {
				out = append(out, p[start:i])
			}
			start = i + 1
		}
	}
	return out
}
