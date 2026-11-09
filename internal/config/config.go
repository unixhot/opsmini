// Package config loads and parses the OpsMini configuration.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Server holds the server listening configuration.
type Server struct {
	Host        string `yaml:"host"`
	Port        int    `yaml:"port"`
	SecretEntry string `yaml:"secret_entry"`
}

// Addr returns the listening address.
func (s Server) Addr() string {
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}

// Database holds the data storage configuration.
type Database struct {
	Path string `yaml:"path"`
}

// JWT holds the JWT authentication configuration.
type JWT struct {
	Secret     string `yaml:"secret"`
	AccessTTL  int    `yaml:"access_ttl_seconds"`
	RefreshTTL int    `yaml:"refresh_ttl_seconds"`
}

// AI holds the AI large language model integration configuration.
type AI struct {
	Enabled bool `yaml:"enabled"`
	// Provider is the wire protocol, not the vendor: openai (covers OpenAI,
	// DeepSeek, Qwen, Moonshot, Zhipu, Ollama, vLLM and one-api relays),
	// anthropic (Claude), azure (Azure OpenAI) or gemini (Google).
	Provider string `yaml:"provider"`
	Model    string `yaml:"model"`
	BaseURL  string `yaml:"base_url"`
	APIKey   string `yaml:"api_key"`
	// AzureAPIVersion is only used when provider=azure.
	AzureAPIVersion string `yaml:"azure_api_version"`
}

// Agent holds the gRPC Agent configuration (去 SaltStack 化).
// The Agent actively dials an OpsAnt "OpsMini Server" over a single gRPC stream.
// Empty ServerAddr disables this feature; the standalone panel is unaffected.
type Agent struct {
	ServerAddr       string `yaml:"server_addr"`       // OpsMini Server address (host:port)
	ServerToken      string `yaml:"server_token"`      // authentication token for the Server
	HeartbeatSeconds int    `yaml:"heartbeat_seconds"` // heartbeat interval in seconds (default 30)
}

// Metrics holds the Prometheus metrics endpoint configuration (node_exporter compatible).
// Authentication uses HTTP Basic auth (username + password), the only scheme node_exporter
// targets support natively via Prometheus `basic_auth`. Empty username means no auth.
type Metrics struct {
	Enabled  bool   `yaml:"enabled"`  // whether to enable the /metrics endpoint
	User     string `yaml:"user"`     // basic auth username; empty means no authentication
	Password string `yaml:"password"` // basic auth password
}

// Paths holds the on-disk layout.
//
// One-directory principle: everything OpsMini writes (reports, change snapshots,
// quarantine copies, plugin packages, log-pipeline config and spool) lives under
// Data. Uninstalling is `rm -rf /data/opsmini`, and there is never a stray file in
// /var/lib or /etc to hunt down afterwards.
type Paths struct {
	// Data is the runtime data root. Empty means "a data/ directory next to the
	// database file", which keeps a default install self-contained without any
	// configuration.
	Data string `yaml:"data"`
}

// BackupTarget is one off-host storage destination.
//
// S3 and SFTP share one flat struct instead of nested blocks: the file is meant to
// be edited by hand, and a nested `s3:` block under a target that only ever uses
// one type is more typing for no clarity.
type BackupTarget struct {
	Name string `yaml:"name"`
	Type string `yaml:"type"` // s3 | sftp

	// S3 / S3-compatible (MinIO, OSS, Ceph, B2)
	Endpoint  string `yaml:"endpoint"`
	Region    string `yaml:"region"`
	Bucket    string `yaml:"bucket"`
	Prefix    string `yaml:"prefix"`
	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`
	PathStyle bool   `yaml:"path_style"`

	// SFTP
	Host           string `yaml:"host"`
	User           string `yaml:"user"`
	Password       string `yaml:"password"`
	PrivateKey     string `yaml:"private_key"`
	BaseDir        string `yaml:"base_dir"`
	KnownHostsFile string `yaml:"known_hosts_file"`
}

// Backup holds the off-host targets available to backup jobs.
type Backup struct {
	Targets []BackupTarget `yaml:"targets"`
}

// Log holds the application logging configuration.
type Log struct {
	Level string `yaml:"level"` // log level: info / warn / error (default error)
	Path  string `yaml:"path"`  // log file path; empty logs to stdout (systemd journal)
}

// Config holds the top-level configuration.
type Config struct {
	Server   Server   `yaml:"server"`
	Database Database `yaml:"database"`
	JWT      JWT      `yaml:"jwt"`
	AI       AI       `yaml:"ai"`
	Agent    Agent    `yaml:"agent"`
	Metrics  Metrics  `yaml:"metrics"`
	Paths    Paths    `yaml:"paths"`
	Backup   Backup   `yaml:"backup"`
	Log      Log      `yaml:"log"`
}

// DataDir returns the runtime data root, resolving the default from the database
// location so that an install is always contained in one directory.
//
// The old code fell back to /var/lib/opsmini/data, which silently scattered files
// outside the install directory whenever the panel setting was empty or cleared.
func (c *Config) DataDir() string {
	if d := strings.TrimSpace(c.Paths.Data); d != "" {
		return filepath.Clean(d)
	}
	db := c.Database.Path
	if db == "" {
		db = "opsmini.db"
	}
	dir := filepath.Dir(db)
	if dir == "" || dir == "." {
		return filepath.Join("data")
	}
	return filepath.Join(dir, "data")
}

// Load loads the configuration from path; returns the default configuration when the file does not exist.
func Load(path string) (*Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// defaultConfig returns the built-in configuration used when no config file is present.
func defaultConfig() *Config {
	return &Config{
		Server: Server{Host: "0.0.0.0", Port: 8888},
		Database: Database{
			Path: "opsmini.db",
		},
		JWT: JWT{
			Secret:     "change-me",
			AccessTTL:  900,
			RefreshTTL: 604800,
		},
		AI: AI{
			Enabled:  true,
			Provider: "openai",
			Model:    "gpt-4o",
			BaseURL:  "https://api.openai.com/v1",
		},
		Metrics: Metrics{
			Enabled: true,
		},
		Log: Log{
			Level: "error",
		},
	}
}
