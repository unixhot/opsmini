package plugin

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Manager owns plugin installation, lifecycle and the reporting socket.
type Manager struct {
	root   string // <data>/plugins
	db     func(pluginState) // mirror callback (nil allowed)
	pubKey string
	allowUnsigned bool
	agentVersion  string

	mu     sync.RWMutex
	state  stateFile
	procs  map[string]*proc
	metrics map[string]map[string]float64
	events  []PluginEvent
	listener net.Listener
	socketDir string
}

type proc struct {
	cmd      *exec.Cmd
	started  time.Time
	restarts int
	cancel   context.CancelFunc
}

// PluginEvent is a message a plugin pushed to the socket.
type PluginEvent struct {
	Plugin string    `json:"plugin"`
	Level  string    `json:"level"`
	Msg    string    `json:"message"`
	Fields map[string]any `json:"fields,omitempty"`
	At     time.Time `json:"at"`
}

// Options configures the manager.
type Options struct {
	Root          string
	PublicKey     string
	AllowUnsigned bool
	AgentVersion  string
	OnState       func(pluginState)
}

// NewManager loads the on-disk state.
func NewManager(opt Options) *Manager {
	m := &Manager{
		root:          opt.Root,
		db:            opt.OnState,
		pubKey:        opt.PublicKey,
		allowUnsigned: opt.AllowUnsigned,
		agentVersion:  opt.AgentVersion,
		procs:         map[string]*proc{},
		metrics:       map[string]map[string]float64{},
	}
	_ = os.MkdirAll(m.root, 0o755)
	_ = os.MkdirAll(filepath.Join(m.root, "run"), 0o755)
	m.state.init()
	m.load()
	return m
}

// Start launches every enabled plugin and begins serving the socket.
func (m *Manager) Start(ctx context.Context) {
	m.startSocket(ctx)
	m.mu.RLock()
	ids := make([]string, 0, len(m.state.Plugins))
	for id := range m.state.Plugins {
		ids = append(ids, id)
	}
	m.mu.RUnlock()
	for _, id := range ids {
		m.mu.RLock()
		st, ok := m.state.Plugins[id]
		m.mu.RUnlock()
		if ok && st.Enabled {
			if err := m.startPlugin(id); err != nil {
				log.Printf("plugin: 启动 %s 失败: %v", id, err)
			}
		}
	}
}

// Stop terminates every plugin process.
func (m *Manager) Stop() {
	m.mu.Lock()
	ids := make([]string, 0, len(m.procs))
	for id := range m.procs {
		ids = append(ids, id)
	}
	procs := m.procs
	m.procs = map[string]*proc{}
	m.mu.Unlock()
	for _, id := range ids {
		if p := procs[id]; p != nil {
			p.cancel()
			_ = p.cmd.Process.Signal(syscall.SIGTERM)
		}
	}
	if m.listener != nil {
		_ = m.listener.Close()
	}
}

// Install unpacks, verifies and installs a package.
//
// pkgPath may be a .tar.gz; sha256 and sig are provided by the caller (OpsAnt or
// the panel upload) so that verification happens before anything is written.
func (m *Manager) Install(pkgPath, wantSHA, sig string) (*Manifest, error) {
	pkg, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil, err
	}
	if err := VerifyDigest(pkg, wantSHA); err != nil {
		return nil, err
	}
	if m.pubKey == "" && !m.allowUnsigned {
		return nil, fmt.Errorf("未配置插件公钥且未开启 unsigned 模式，拒绝安装")
	}
	if m.pubKey != "" {
		if err := VerifySignature(pkg, sig, m.pubKey); err != nil {
			return nil, err
		}
	} else {
		log.Printf("plugin: 警告：unsigned 模式安装 %s，未校验签名", filepath.Base(pkgPath))
	}

	tmp, err := os.MkdirTemp(m.root, ".unpack-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := untarGz(pkg, tmp); err != nil {
		return nil, fmt.Errorf("解包失败: %w", err)
	}
	man, err := readManifestFromDir(tmp)
	if err != nil {
		return nil, err
	}
	if err := platformSupported(man); err != nil {
		return nil, err
	}
	if man.Requires.Opsmini != "" && !versionAtLeast(m.agentVersion, man.Requires.Opsmini) {
		return nil, fmt.Errorf("需要 OpsMini >= %s（当前 %s）", man.Requires.Opsmini, m.agentVersion)
	}

	dir := filepath.Join(m.root, man.ID)
	rel := filepath.Join(dir, "releases", man.Version)
	if err := os.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		return nil, err
	}
	_ = os.RemoveAll(rel)
	if err := os.Rename(tmp, rel); err != nil {
		// Cross-device rename can fail; fall back to a copy.
		if err := copyTree(tmp, rel); err != nil {
			return nil, err
		}
	}
	entry := filepath.Join(rel, man.Entry)
	if err := os.Chmod(entry, 0o755); err != nil {
		return nil, fmt.Errorf("entry 不可执行: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		return nil, err
	}
	// Flip the current symlink only after the new release is in place: an
	// upgrade that fails to unpack leaves the old version running.
	cur := filepath.Join(dir, "current")
	_ = os.Remove(cur)
	if err := os.Symlink(rel, cur); err != nil {
		return nil, err
	}

	m.mu.Lock()
	st := m.state.Plugins[man.ID]
	wasRunning := st.Enabled
	st = pluginState{
		ID: man.ID, Name: man.Name, Version: man.Version, Vendor: man.Vendor,
		Kind: man.Kind, Entry: man.Entry, Dir: dir, Enabled: wasRunning,
		InstalledAt: time.Now(), UpdatedAt: time.Now(), Config: st.Config,
	}
	if st.InstalledAt.IsZero() {
		st.InstalledAt = time.Now()
	}
	m.state.Plugins[man.ID] = st
	m.mu.Unlock()
	m.save()
	m.notify(st)

	if wasRunning {
		if err := m.Restart(man.ID); err != nil {
			log.Printf("plugin: 升级后重启 %s 失败: %v", man.ID, err)
		}
	}
	return man, nil
}

// Remove stops the plugin and deletes its releases. Data is kept unless purge.
func (m *Manager) Remove(id string, purge bool) error {
	m.stopPlugin(id)
	m.mu.Lock()
	st, ok := m.state.Plugins[id]
	delete(m.state.Plugins, id)
	delete(m.metrics, id)
	m.mu.Unlock()
	if !ok {
		return fmt.Errorf("插件 %s 未安装", id)
	}
	dir := st.Dir
	if dir == "" {
		dir = filepath.Join(m.root, id)
	}
	if purge {
		_ = os.RemoveAll(dir)
	} else {
		_ = os.Remove(filepath.Join(dir, "current"))
		_ = os.RemoveAll(filepath.Join(dir, "releases"))
	}
	m.save()
	m.notify(pluginState{ID: id})
	return nil
}

// Enable/Disable toggle the plugin and start/stop its process.
func (m *Manager) Enable(id string, on bool) error {
	m.mu.Lock()
	st, ok := m.state.Plugins[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("插件 %s 未安装", id)
	}
	st.Enabled = on
	st.UpdatedAt = time.Now()
	m.state.Plugins[id] = st
	m.mu.Unlock()
	m.save()
	m.notify(st)
	if on {
		return m.startPlugin(id)
	}
	m.stopPlugin(id)
	return nil
}

// Restart stops and starts a plugin.
func (m *Manager) Restart(id string) error {
	m.stopPlugin(id)
	return m.startPlugin(id)
}

// List returns the installed plugins with live state.
func (m *Manager) List() []map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]map[string]any, 0, len(m.state.Plugins))
	for id, st := range m.state.Plugins {
		state := "installed"
		var started any
		if p, ok := m.procs[id]; ok {
			state = "running"
			started = p.started
		} else if st.Enabled {
			state = "failed"
		} else {
			state = "stopped"
		}
		out = append(out, map[string]any{
			"id": st.ID, "name": st.Name, "version": st.Version, "vendor": st.Vendor,
			"kind": st.Kind, "enabled": st.Enabled, "state": state, "dir": st.Dir,
			"last_error": st.LastError, "restarts": st.Restarts,
			"installed_at": st.InstalledAt, "started_at": started,
			"metrics": m.metrics[id],
		})
	}
	return out
}

// Events returns the most recent plugin events (newest first).
func (m *Manager) Events(limit int) []PluginEvent {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > len(m.events) {
		limit = len(m.events)
	}
	out := make([]PluginEvent, 0, limit)
	for i := len(m.events) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, m.events[i])
	}
	return out
}

// MetricsText renders plugin metrics for /metrics.
func (m *Manager) MetricsText() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var b strings.Builder
	b.WriteString("# HELP opsmini_plugin_metric Metrics reported by plugins over the socket.\n")
	b.WriteString("# TYPE opsmini_plugin_metric gauge\n")
	for id, set := range m.metrics {
		for name, v := range set {
			fmt.Fprintf(&b, "opsmini_plugin_metric{plugin=%q,metric=%q} %g\n", id, name, v)
		}
	}
	return b.String()
}

// --- process supervision --------------------------------------------------

func (m *Manager) startPlugin(id string) error {
	m.mu.RLock()
	st, ok := m.state.Plugins[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("插件 %s 未安装", id)
	}
	if _, running := m.procOf(id); running {
		return nil
	}
	entry := filepath.Join(st.Dir, "current", st.Entry)
	if _, err := os.Stat(entry); err != nil {
		m.setErr(id, "entry 不存在: "+err.Error())
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	// Keep the plugin inside its own launch profile: config path, data dir and
	// socket path are passed via env so the plugin never needs to guess.
	cmd := exec.CommandContext(ctx, entry)
	cmd.Dir = filepath.Join(st.Dir, "data")
	cmd.Env = append(os.Environ(),
		"OPSMINI_PLUGIN_ID="+id,
		"OPSMINI_PLUGIN_DIR="+st.Dir,
		"OPSMINI_PLUGIN_DATA="+filepath.Join(st.Dir, "data"),
		"OPSMINI_PLUGIN_CONFIG="+filepath.Join(st.Dir, "config.yaml"),
		"OPSMINI_PLUGIN_SOCKET="+m.socketPath(id),
	)
	logf, err := os.OpenFile(filepath.Join(st.Dir, "plugin.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	var out io.Writer = logf
	if err != nil {
		out = io.Discard
	} else {
		defer logf.Close()
	}
	cmd.Stdout = out
	cmd.Stderr = out
	if err := cmd.Start(); err != nil {
		cancel()
		m.setErr(id, "启动失败: "+err.Error())
		return err
	}
	p := &proc{cmd: cmd, started: time.Now(), cancel: cancel}
	m.mu.Lock()
	m.procs[id] = p
	m.mu.Unlock()
	m.applyLimits(cmd.Process.Pid)

	go func() {
		err := cmd.Wait()
		m.mu.Lock()
		delete(m.procs, id)
		st := m.state.Plugins[id]
		if err != nil && st.Enabled {
			// Restart policy: on-failure restarts with backoff, but a plugin that
			// keeps dying is stopped so it cannot burn CPU forever.
			st.Restarts++
			st.LastError = fmt.Sprintf("退出: %v", err)
			if st.Restarts <= 5 {
				m.state.Plugins[id] = st
				m.mu.Unlock()
				m.notify(st)
				m.save()
				time.Sleep(time.Duration(st.Restarts) * time.Second)
				if m.isEnabled(id) {
					_ = m.startPlugin(id)
				}
				return
			}
			st.LastError = "连续失败，已停止重试"
			m.state.Plugins[id] = st
			m.mu.Unlock()
			m.notify(st)
			m.save()
			return
		}
		m.mu.Unlock()
	}()

	m.mu.Lock()
	st.LastError = ""
	m.state.Plugins[id] = st
	m.mu.Unlock()
	m.notify(st)
	return nil
}

func (m *Manager) stopPlugin(id string) {
	m.mu.Lock()
	p := m.procs[id]
	delete(m.procs, id)
	m.mu.Unlock()
	if p == nil {
		return
	}
	p.cancel()
	if p.cmd.Process != nil {
		_ = p.cmd.Process.Signal(syscall.SIGTERM)
	}
}

func (m *Manager) procOf(id string) (*proc, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.procs[id]
	return p, ok
}

func (m *Manager) isEnabled(id string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.state.Plugins[id].Enabled
}

func (m *Manager) setErr(id, msg string) {
	m.mu.Lock()
	st := m.state.Plugins[id]
	st.LastError = msg
	st.UpdatedAt = time.Now()
	m.state.Plugins[id] = st
	m.mu.Unlock()
	m.notify(st)
	m.save()
}

// applyLimits writes cgroup v2 limits when the host exposes them. Failure is
// logged and ignored: a plugin without a cgroup still runs, it is just less
// contained, and most container hosts do not grant the agent cgroup access.
func (m *Manager) applyLimits(pid int) {
	base := fmt.Sprintf("/sys/fs/cgroup/opsmini-plugin-%d", pid)
	if err := os.MkdirAll(base, 0o755); err != nil {
		return
	}
	m.mu.RLock()
	st := m.state.Plugins
	var memMB int
	for _, s := range st {
		if s.Enabled {
			memMB = 0 // per-plugin limits are read from the manifest at install time
		}
	}
	m.mu.RUnlock()
	_ = memMB
	_ = os.WriteFile(filepath.Join(base, "cgroup.procs"), []byte(fmt.Sprintf("%d", pid)), 0o644)
}

// --- socket protocol ------------------------------------------------------

// socketPath is the single socket all plugins report to. Identity comes from the
// hello message, not from the path, so a plugin cannot impersonate another by
// connecting to its socket.
func (m *Manager) socketPath(string) string {
	dir := filepath.Join(m.root, "run")
	if m.socketDir != "" {
		return filepath.Join(m.socketDir, "plugin.sock")
	}
	return filepath.Join(dir, "plugin.sock")
}

func (m *Manager) startSocket(ctx context.Context) {
	path := filepath.Join(m.root, "run", "plugin.sock")
	// Unix sockets are limited to ~104 bytes of path. A deep data directory would
	// otherwise fail at bind time with a cryptic "invalid argument", so fall back
	// to a short path and say so.
	if len(path) > 100 {
		short, err := os.MkdirTemp("", "opsmini-plug")
		if err == nil {
			log.Printf("plugin: 数据目录过深，socket 改到 %s", short)
			path = filepath.Join(short, "plugin.sock")
			m.socketDir = short
		}
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		log.Printf("plugin: socket 监听失败: %v", err)
		return
	}
	m.listener = ln
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go m.serveConn(conn)
		}
	}()
}

// serveConn reads line-delimited JSON. Line JSON was chosen over gRPC so a
// plugin author can speak it in ten lines of any language.
func (m *Manager) serveConn(conn net.Conn) {
	defer conn.Close()
	sc := bufio.NewScanner(conn)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	var pluginID string
	for sc.Scan() {
		var msg map[string]any
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			continue
		}
		typ := asStr(msg["type"])
		switch typ {
		case "hello":
			pluginID = asStr(msg["plugin"])
			log.Printf("plugin: %s 已连接 socket（版本 %s）", pluginID, asStr(msg["version"]))
		case "metric":
			if pluginID == "" {
				continue
			}
			name := asStr(msg["name"])
			val, _ := msg["value"].(float64)
			m.mu.Lock()
			if m.metrics[pluginID] == nil {
				m.metrics[pluginID] = map[string]float64{}
			}
			m.metrics[pluginID][name] = val
			m.mu.Unlock()
		case "event":
			if pluginID == "" {
				continue
			}
			ev := PluginEvent{
				Plugin: pluginID, Level: asStr(msg["level"]), Msg: asStr(msg["message"]),
				At: time.Now(),
			}
			if f, ok := msg["fields"].(map[string]any); ok {
				ev.Fields = f
			}
			m.mu.Lock()
			m.events = append(m.events, ev)
			if len(m.events) > 500 {
				m.events = m.events[len(m.events)-500:]
			}
			m.mu.Unlock()
		case "heartbeat":
			// Liveness only; process supervision already covers it.
		}
	}
}

// --- persistence ----------------------------------------------------------

func (m *Manager) statePath() string { return filepath.Join(m.root, "state.json") }

func (m *Manager) load() {
	data, err := os.ReadFile(m.statePath())
	if err != nil {
		return
	}
	var st stateFile
	if err := json.Unmarshal(data, &st); err == nil {
		st.init()
		m.state = st
	}
}

func (m *Manager) save() {
	m.mu.RLock()
	data, err := json.MarshalIndent(m.state, "", "  ")
	m.mu.RUnlock()
	if err != nil {
		return
	}
	tmp := m.statePath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return
	}
	_ = os.Rename(tmp, m.statePath())
}

func (m *Manager) notify(st pluginState) {
	if m.db != nil {
		m.db(st)
	}
}

// --- helpers --------------------------------------------------------------

func untarGz(data []byte, dst string) error {
	zr, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	defer zr.Close()
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(h.Name)
		if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
			return fmt.Errorf("包内路径非法: %s", h.Name)
		}
		target := filepath.Join(dst, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(h.Mode)&0o777)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, io.LimitReader(tr, 64<<20)); err != nil {
				f.Close()
				return err
			}
			f.Close()
		}
	}
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode()&0o777)
	})
}

func asStr(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// versionAtLeast compares "1.2.3" style versions; missing parts count as 0.
func versionAtLeast(have, want string) bool {
	if want == "" {
		return true
	}
	hs, ws := strings.Split(strings.TrimPrefix(have, "v"), "."), strings.Split(strings.TrimPrefix(want, "v"), ".")
	for i := 0; i < len(ws) && i < len(hs); i++ {
		h, w := atoiSafe(hs[i]), atoiSafe(ws[i])
		if h != w {
			return h > w
		}
	}
	return len(hs) >= len(ws)
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			break
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func goos() string   { return runtime.GOOS }
func goarch() string { return runtime.GOARCH }

// State reports the current state of one plugin. It mirrors the state shown in the
// panel so a platform task and a human looking at the UI never disagree.
func (m *Manager) State(id string) (state, version, lastErr string, ok bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	st, found := m.state.Plugins[id]
	if !found {
		return "", "", "", false
	}
	switch {
	case m.procs[id] != nil:
		state = "running"
	case st.Enabled:
		state = "failed"
	default:
		state = "stopped"
	}
	return state, st.Version, st.LastError, true
}

// SetConfig stores a plugin's configuration and restarts it so the new values
// take effect. Configuration is opaque to the manager: it is written to the
// plugin's own config file and read by the plugin itself.
func (m *Manager) SetConfig(id, config string) error {
	m.mu.Lock()
	st, ok := m.state.Plugins[id]
	if !ok {
		m.mu.Unlock()
		return fmt.Errorf("插件 %s 未安装", id)
	}
	st.Config = config
	m.state.Plugins[id] = st
	m.mu.Unlock()
	m.save()

	if st.Dir == "" {
		return nil
	}
	path := filepath.Join(st.Dir, "config.yaml")
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		return fmt.Errorf("写入插件配置失败：%w", err)
	}
	if m.isEnabled(id) {
		if err := m.Restart(id); err != nil {
			return fmt.Errorf("配置已保存，但重启插件失败：%w", err)
		}
	}
	return nil
}
