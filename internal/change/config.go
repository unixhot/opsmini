package change

import "time"

// Config controls the change awareness subsystem.
type Config struct {
	Enabled  bool
	File     FileConfig
	Package  PackageConfig
	Service  ServiceConfig
	Account  AccountConfig
	Firewall FirewallConfig
	Cron     CronConfig
	Snapshot SnapshotConfig
	Retention RetentionConfig
	Quota    QuotaConfig

	// ActorResolver lets the file source attribute changes to an active session.
	// It returns ok=false when no session can be reasonably associated; the
	// caller must then report confidence=unknown rather than guessing.
	ActorResolver func(at time.Time) (Actor, bool)
}

// FileConfig configures configuration-file watching.
type FileConfig struct {
	WatchDirs      []string
	Realtime       bool
	ScanInterval   time.Duration
	MaxFileSize    int64
	MaxDiffSize    int64
	IgnoreGlobs    []string
	ExtraBlocked   []string // can only ADD to the hard-coded blocklist
}

// PackageConfig configures dpkg/apt/dnf log parsing.
//
// SnapshotInterval is the fallback cadence, not the primary path: many cloud
// images ship an empty /var/log/dpkg.log (logrotate keeps only .1/.gz), so the
// installed-package set is compared periodically as a safety net. Log tailing
// runs independently every 30s so a real install still shows up immediately.
type PackageConfig struct {
	Enabled          bool
	SnapshotInterval time.Duration
}

// ServiceConfig configures systemd unit polling.
type ServiceConfig struct {
	Enabled      bool
	PollInterval time.Duration
}

// AccountConfig configures passwd/group/sudoers watching.
type AccountConfig struct {
	Enabled bool
}

// FirewallConfig configures firewall rule diffing.
type FirewallConfig struct {
	Enabled      bool
	PollInterval time.Duration
}

// CronConfig configures scheduled job watching.
type CronConfig struct {
	Enabled bool
}

// SnapshotConfig bounds the "before" copies kept for rollback.
type SnapshotConfig struct {
	Enabled      bool
	MaxPerDay    int
	MaxTotalSize int64
	Dir          string
}

// RetentionConfig keeps metadata longer than bodies on purpose.
type RetentionConfig struct {
	MetadataDays int
	DiffDays     int
}

// QuotaConfig bounds diff storage.
type QuotaConfig struct {
	DiffTotalSize int64
}

// DefaultConfig returns the configuration used when the panel has none.
func DefaultConfig() *Config {
	return &Config{
		Enabled: true,
		File: FileConfig{
			WatchDirs:    []string{"/etc", "/usr/local/etc"},
			Realtime:     true,
			ScanInterval: 5 * time.Minute,
			MaxFileSize:  maxTrackedFileSize,
			MaxDiffSize:  maxDiffBytes,
			IgnoreGlobs: []string{
				"/etc/mtab", "/etc/ld.so.cache", "/etc/adjtime",
				"/etc/resolv.conf", "/etc/hosts.allow", "/etc/hosts.deny",
			},
		},
		Package:  PackageConfig{Enabled: true, SnapshotInterval: 5 * time.Minute},
		Service:  ServiceConfig{Enabled: true, PollInterval: 60 * time.Second},
		Account:  AccountConfig{Enabled: true},
		Firewall: FirewallConfig{Enabled: true, PollInterval: 5 * time.Minute},
		Cron:     CronConfig{Enabled: true},
		Snapshot: SnapshotConfig{Enabled: true, MaxPerDay: 200, MaxTotalSize: 500 * 1024 * 1024},
		Retention: RetentionConfig{
			MetadataDays: 180,
			DiffDays:     30,
		},
		Quota: QuotaConfig{DiffTotalSize: 200 * 1024 * 1024},
	}
}

func (c *Config) normalize() {
	d := DefaultConfig()
	if c.File.ScanInterval <= 0 {
		c.File.ScanInterval = d.File.ScanInterval
	}
	if len(c.File.WatchDirs) == 0 {
		c.File.WatchDirs = d.File.WatchDirs
	}
	if c.File.MaxFileSize <= 0 {
		c.File.MaxFileSize = d.File.MaxFileSize
	}
	if c.Service.PollInterval <= 0 {
		c.Service.PollInterval = d.Service.PollInterval
	}
	if c.Firewall.PollInterval <= 0 {
		c.Firewall.PollInterval = d.Firewall.PollInterval
	}
	if c.Package.SnapshotInterval <= 0 {
		c.Package.SnapshotInterval = d.Package.SnapshotInterval
	}
	if c.Retention.MetadataDays <= 0 {
		c.Retention.MetadataDays = d.Retention.MetadataDays
	}
	if c.Retention.DiffDays <= 0 {
		c.Retention.DiffDays = d.Retention.DiffDays
	}
	if c.Quota.DiffTotalSize <= 0 {
		c.Quota.DiffTotalSize = d.Quota.DiffTotalSize
	}
	if c.Snapshot.MaxPerDay <= 0 {
		c.Snapshot.MaxPerDay = d.Snapshot.MaxPerDay
	}
	if c.Snapshot.MaxTotalSize <= 0 {
		c.Snapshot.MaxTotalSize = d.Snapshot.MaxTotalSize
	}
}
