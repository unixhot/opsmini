// Package plugin implements the OpsMini plugin runtime: package verification,
// installation, lifecycle and the local socket protocol plugins use to report
// metrics and events.
//
// Design constraints (docs/v1.4-plugin-system.md):
//   - plugins are separate processes, never Go plugins (a crashed plugin must not
//     take the agent down, and an OpsMini upgrade must not invalidate them);
//   - packages are signed (ed25519) and the signature is verified before unpack;
//   - the kernel exposes nothing to a plugin except a unix socket, a config file
//     and a data directory.
package plugin

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Manifest is plugin.yaml, the only thing the runtime trusts about a package.
type Manifest struct {
	APIVersion   string   `yaml:"apiVersion"`
	ID           string   `yaml:"id"`
	Name         string   `yaml:"name"`
	Version      string   `yaml:"version"`
	Vendor       string   `yaml:"vendor"`
	Description  string   `yaml:"description"`
	Kind         string   `yaml:"kind"`
	Entry        string   `yaml:"entry"`
	Platform     []string `yaml:"platform"`
	Capabilities []string `yaml:"capabilities"`
	Runtime      Runtime  `yaml:"runtime"`
	Requires     Requires `yaml:"requires"`
}

// Runtime holds the process constraints.
type Runtime struct {
	User            string `yaml:"user"`
	MemoryLimitMB   int    `yaml:"memoryLimitMB"`
	CPUQuotaPercent int    `yaml:"cpuQuotaPercent"`
	RestartPolicy   string `yaml:"restartPolicy"`
	Socket          string `yaml:"socket"`
}

// Requires declares the minimum agent version.
type Requires struct {
	Opsmini string `yaml:"opsmini"`
}

const (
	apiVersion = "opsmini.plugin/v1"
	// SignatureFile is the detached ed25519 signature over the package digest.
	SignatureFile = "SIGNATURE"
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,40}$`)

// ParseManifest decodes and validates plugin.yaml.
func ParseManifest(data []byte) (*Manifest, error) {
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("plugin.yaml 解析失败: %w", err)
	}
	if m.APIVersion != apiVersion {
		return nil, fmt.Errorf("apiVersion 必须为 %s（实际 %q）", apiVersion, m.APIVersion)
	}
	if !idPattern.MatchString(m.ID) {
		return nil, fmt.Errorf("插件 id 不合法: %q", m.ID)
	}
	if m.Version == "" || m.Name == "" {
		return nil, fmt.Errorf("插件缺少 name/version")
	}
	switch m.Kind {
	case "collector", "action", "panel":
	default:
		return nil, fmt.Errorf("kind 必须是 collector/action/panel（实际 %q）", m.Kind)
	}
	if m.Entry == "" {
		return nil, fmt.Errorf("插件缺少 entry")
	}
	// The entry must stay inside the package: "../.." in a manifest is a path
	// traversal attempt, not a layout choice.
	clean := filepath.Clean(m.Entry)
	if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return nil, fmt.Errorf("entry 必须是包内相对路径")
	}
	m.Entry = clean
	switch m.Runtime.RestartPolicy {
	case "", "always", "on-failure", "never":
	default:
		return nil, fmt.Errorf("restartPolicy 只能是 always/on-failure/never")
	}
	if m.Runtime.MemoryLimitMB < 0 || m.Runtime.CPUQuotaPercent < 0 {
		return nil, fmt.Errorf("资源限制不能为负")
	}
	return &m, nil
}

// VerifyDigest checks the package sha256 against the value the caller was given.
func VerifyDigest(pkg []byte, want string) error {
	if want == "" {
		return fmt.Errorf("缺少包摘要（sha256），拒绝安装")
	}
	sum := sha256.Sum256(pkg)
	got := hex.EncodeToString(sum[:])
	if !strings.EqualFold(got, want) {
		return fmt.Errorf("包摘要不匹配：期望 %s，实际 %s", want, got)
	}
	return nil
}

// VerifySignature checks the detached ed25519 signature (base64) over the same
// digest with the given public key (base64 or hex, 32 bytes).
//
// An empty key means "unsigned mode": only allowed when the operator explicitly
// opted in, and the caller logs it loudly.
func VerifySignature(pkg []byte, sigB64, pubB64 string) error {
	if pubB64 == "" {
		return fmt.Errorf("未配置插件公钥，无法校验签名")
	}
	pub, err := decodeKey(pubB64)
	if err != nil {
		return fmt.Errorf("插件公钥无效: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(strings.TrimSpace(sigB64))
	if err != nil {
		return fmt.Errorf("签名不是合法 base64: %w", err)
	}
	sum := sha256.Sum256(pkg)
	if !ed25519.Verify(ed25519.PublicKey(pub), sum[:], sig) {
		return fmt.Errorf("签名校验失败：包被篡改或不是用该密钥签发")
	}
	return nil
}

func decodeKey(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s)); err == nil && len(b) == ed25519.PublicKeySize {
		return b, nil
	}
	if b, err := hex.DecodeString(strings.TrimSpace(s)); err == nil && len(b) == ed25519.PublicKeySize {
		return b, nil
	}
	return nil, fmt.Errorf("长度不是 %d 字节", ed25519.PublicKeySize)
}

// SignPackage is used by the packaging tool / tests: it produces the base64
// signature for a package with a private key.
func SignPackage(pkg []byte, priv ed25519.PrivateKey) string {
	sum := sha256.Sum256(pkg)
	return base64.StdEncoding.EncodeToString(ed25519.Sign(priv, sum[:]))
}

// platformSupported checks the manifest against the running platform.
func platformSupported(m *Manifest) error {
	if len(m.Platform) == 0 {
		return nil
	}
	want := goos() + "/" + goarch()
	for _, p := range m.Platform {
		if strings.TrimSpace(p) == want {
			return nil
		}
	}
	return fmt.Errorf("插件不支持当前平台（需要 %v，本机 %s）", m.Platform, want)
}

// stateFile is the on-disk source of truth for what is installed.
type stateFile struct {
	Plugins map[string]pluginState `json:"plugins"`
}

type pluginState struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Vendor      string    `json:"vendor"`
	Kind        string    `json:"kind"`
	Entry       string    `json:"entry"`
	Dir         string    `json:"dir"`
	Enabled     bool      `json:"enabled"`
	Restarts    int       `json:"restarts"`
	LastError   string    `json:"last_error,omitempty"`
	InstalledAt time.Time `json:"installed_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Config is the plugin's own configuration (opaque YAML to us).
	Config string `json:"config,omitempty"`
}

func (s *stateFile) init() {
	if s.Plugins == nil {
		s.Plugins = map[string]pluginState{}
	}
}

// readManifestFromDir loads and validates the manifest inside an unpacked dir.
func readManifestFromDir(dir string) (*Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "plugin.yaml"))
	if err != nil {
		return nil, fmt.Errorf("缺少 plugin.yaml: %w", err)
	}
	return ParseManifest(data)
}
