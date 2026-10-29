package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const goodManifest = `apiVersion: opsmini.plugin/v1
id: demo-collector
name: Demo 采集器
version: 1.0.0
vendor: 速云科技
kind: collector
entry: bin/plugin
platform: [linux/amd64, darwin/arm64]
capabilities: [metrics]
runtime:
  restartPolicy: on-failure
`

func TestParseManifestValidates(t *testing.T) {
	m, err := ParseManifest([]byte(goodManifest))
	if err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
	if m.ID != "demo-collector" || m.Kind != "collector" {
		t.Fatalf("unexpected manifest: %+v", m)
	}

	bad := []struct{ name, yaml string }{
		{"bad apiVersion", strings.Replace(goodManifest, "opsmini.plugin/v1", "v0", 1)},
		{"bad id", strings.Replace(goodManifest, "demo-collector", "Bad_ID", 1)},
		{"bad kind", strings.Replace(goodManifest, "kind: collector", "kind: shell", 1)},
		{"path traversal", strings.Replace(goodManifest, "bin/plugin", "../../etc/passwd", 1)},
		{"bad restart", strings.Replace(goodManifest, "on-failure", "sometimes", 1)},
	}
	for _, tc := range bad {
		if _, err := ParseManifest([]byte(tc.yaml)); err == nil {
			t.Fatalf("%s: expected rejection", tc.name)
		}
	}
}

func TestVerifyDigestAndSignature(t *testing.T) {
	pkg := []byte("package-bytes")
	sum := sha256.Sum256(pkg)
	digest := hex.EncodeToString(sum[:])
	if err := VerifyDigest(pkg, digest); err != nil {
		t.Fatalf("digest should match: %v", err)
	}
	if err := VerifyDigest(pkg, strings.Repeat("0", 64)); err == nil {
		t.Fatalf("digest mismatch must fail")
	}
	if err := VerifyDigest(pkg, ""); err == nil {
		t.Fatalf("missing digest must fail")
	}

	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	sig := SignPackage(pkg, priv)
	if err := VerifySignature(pkg, sig, base64.StdEncoding.EncodeToString(pub)); err != nil {
		t.Fatalf("valid signature rejected: %v", err)
	}
	if err := VerifySignature([]byte("tampered"), sig, base64.StdEncoding.EncodeToString(pub)); err == nil {
		t.Fatalf("tampered package must fail")
	}
}

// buildPackage creates a minimal plugin tar.gz and returns its bytes.
func buildPackage(t *testing.T, scriptBody string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	files := map[string]string{
		"plugin.yaml": goodManifest,
		"bin/plugin": scriptBody,
	}
	names := []string{"plugin.yaml", "bin/plugin"}
	for _, name := range names {
		body := files[name]
		h := &tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func newTestManager(t *testing.T, unsigned bool) *Manager {
	t.Helper()
	// Unix socket paths are capped at ~104 bytes; a deep t.TempDir() path blows
	// that limit on macOS, so tests use a short directory.
	root, err := os.MkdirTemp("/tmp", "pl")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	return NewManager(Options{Root: root, AllowUnsigned: unsigned, AgentVersion: "1.4.0"})
}

func TestInstallWritesStateAndCurrent(t *testing.T) {
	m := newTestManager(t, true)
	pkg := buildPackage(t, "#!/bin/sh\nsleep 60\n")
	pkgPath := filepath.Join(t.TempDir(), "demo.tar.gz")
	if err := os.WriteFile(pkgPath, pkg, 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(pkg)

	man, err := m.Install(pkgPath, hex.EncodeToString(sum[:]), "")
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if man.ID != "demo-collector" {
		t.Fatalf("manifest id = %s", man.ID)
	}
	// On-disk layout: state.json + releases/<version> + current symlink.
	if _, err := os.Stat(filepath.Join(m.root, "state.json")); err != nil {
		t.Fatalf("state.json missing: %v", err)
	}
	cur, err := os.Readlink(filepath.Join(m.root, "demo-collector", "current"))
	if err != nil || !strings.HasSuffix(cur, "1.0.0") {
		t.Fatalf("current symlink = %q (%v)", cur, err)
	}
	if _, err := os.Stat(filepath.Join(m.root, "demo-collector", "current", "bin", "plugin")); err != nil {
		t.Fatalf("entry not installed: %v", err)
	}

	// A second manager must see the same state (state.json is the truth).
	m2 := NewManager(Options{Root: m.root, AllowUnsigned: true, AgentVersion: "1.4.0"})
	list := m2.List()
	if len(list) != 1 || list[0]["id"] != "demo-collector" {
		t.Fatalf("state not reloaded: %v", list)
	}
}

func TestInstallRejectsBadDigest(t *testing.T) {
	m := newTestManager(t, true)
	pkg := buildPackage(t, "#!/bin/sh\nexit 0\n")
	pkgPath := filepath.Join(t.TempDir(), "demo.tar.gz")
	_ = os.WriteFile(pkgPath, pkg, 0o600)
	if _, err := m.Install(pkgPath, strings.Repeat("a", 64), ""); err == nil {
		t.Fatalf("wrong digest must be rejected")
	}
}

func TestInstallRequiresKeyUnlessUnsigned(t *testing.T) {
	m := newTestManager(t, false)
	pkg := buildPackage(t, "#!/bin/sh\nexit 0\n")
	pkgPath := filepath.Join(t.TempDir(), "demo.tar.gz")
	_ = os.WriteFile(pkgPath, pkg, 0o600)
	sum := sha256.Sum256(pkg)
	if _, err := m.Install(pkgPath, hex.EncodeToString(sum[:]), ""); err == nil {
		t.Fatalf("missing public key must be rejected in signed mode")
	}
}

func TestEnableStartsAndDisableStops(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("sh not available")
	}
	m := newTestManager(t, true)
	pkg := buildPackage(t, "#!/bin/sh\nsleep 30\n")
	pkgPath := filepath.Join(t.TempDir(), "demo.tar.gz")
	_ = os.WriteFile(pkgPath, pkg, 0o600)
	sum := sha256.Sum256(pkg)
	if _, err := m.Install(pkgPath, hex.EncodeToString(sum[:]), ""); err != nil {
		t.Fatalf("install: %v", err)
	}
	if err := m.Enable("demo-collector", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, running := m.procOf("demo-collector"); !running {
		t.Fatalf("plugin should be running after enable")
	}
	if err := m.Enable("demo-collector", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, running := m.procOf("demo-collector"); running {
		t.Fatalf("plugin should be stopped after disable")
	}
	m.Stop()
}

// The socket is the plugin/kernel contract: line JSON with metric and event.
func TestSocketAcceptsMetricsAndEvents(t *testing.T) {
	m := newTestManager(t, true)
	m.startSocket(t.Context())
	time.Sleep(100 * time.Millisecond)

	sock := filepath.Join(m.root, "run", "plugin.sock")
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial socket: %v", err)
	}
	defer conn.Close()
	_, _ = conn.Write([]byte(`{"type":"hello","plugin":"demo-collector","version":"1.0.0"}` + "\n"))
	_, _ = conn.Write([]byte(`{"type":"metric","name":"queue_depth","value":42}` + "\n"))
	_, _ = conn.Write([]byte(`{"type":"event","level":"warn","message":"lag high","fields":{"lag":5.2}}` + "\n"))
	time.Sleep(300 * time.Millisecond)

	m.mu.RLock()
	got := m.metrics["demo-collector"]["queue_depth"]
	m.mu.RUnlock()
	if got != 42 {
		t.Fatalf("metric not recorded: %v", got)
	}
	evs := m.Events(10)
	if len(evs) != 1 || evs[0].Msg != "lag high" {
		t.Fatalf("event not recorded: %+v", evs)
	}
	text := m.MetricsText()
	if !strings.Contains(text, `opsmini_plugin_metric{plugin="demo-collector",metric="queue_depth"} 42`) {
		t.Fatalf("metrics text missing plugin metric: %s", text)
	}
}

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		have, want string
		ok         bool
	}{
		{"1.4.0", "1.4.0", true},
		{"1.4.0", "1.3.9", true},
		{"v1.4.0", "1.4", true},
		{"1.4.0", "1.5.0", false},
		{"1.4.0", "", true},
	}
	for _, c := range cases {
		if got := versionAtLeast(c.have, c.want); got != c.ok {
			t.Fatalf("versionAtLeast(%q,%q) = %v, want %v", c.have, c.want, got, c.ok)
		}
	}
}
