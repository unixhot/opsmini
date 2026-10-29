package model

import "time"

// Change kinds (one per change source).
const (
	ChangeKindConfig   = "config"   // 配置文件内容变化
	ChangeKindPackage  = "package"  // 软件包安装/升级/卸载
	ChangeKindService  = "service"  // systemd 单元状态变化
	ChangeKindAccount  = "account"  // 账号与权限变化
	ChangeKindFirewall = "firewall" // 防火墙规则变化
	ChangeKindCron     = "cron"     // 定时任务变化
)

// Actor attribution confidence. Never upgrade "inferred" to "confirmed"
// without hard evidence (audit log / panel record) — mis-attribution
// destroys trust faster than missing attribution.
const (
	ConfidenceConfirmed = "confirmed"
	ConfidenceInferred  = "inferred"
	ConfidenceUnknown   = "unknown"
)

// ChangeEvent is the record of "something on this host was modified".
//
// It is deliberately separate from Signal: a Signal is an observation of an
// anomaly and may trigger an agent run, whereas a ChangeEvent is a statement of
// fact. Change events NEVER trigger writes (enforced in the trigger layer); they
// exist to give diagnosis a causal starting point.
//
// Metadata lives here (kept long); the diff body lives in ChangeDiff (kept
// short), because repeated queries need the metadata while the body is both
// expensive and sensitive.
type ChangeEvent struct {
	ID         string `gorm:"primaryKey;size:40" json:"id"`
	Kind       string `gorm:"size:16;index:idx_chg_kind_at,priority:1" json:"kind"`
	Source     string `gorm:"size:64" json:"source"`
	ObjectType string `gorm:"size:16" json:"object_type"`
	ObjectKey  string `gorm:"size:512;index:idx_chg_key_at,priority:1" json:"object_key"`
	Labels     string `gorm:"size:512" json:"labels"` // 逗号分隔，用于关联打分

	ActorKind  string `gorm:"size:16" json:"actor_kind"`
	ActorUser  string `gorm:"size:64" json:"actor_user"`
	ActorIP    string `gorm:"size:64" json:"actor_ip"`
	ActorHow   string `gorm:"size:128" json:"actor_how"`
	Confidence string `gorm:"size:16" json:"confidence"`

	Added     int    `json:"added"`
	Removed   int    `json:"removed"`
	Fields    string `gorm:"size:512" json:"fields"`
	HashFrom  string `gorm:"size:64" json:"hash_from"`
	HashTo    string `gorm:"size:64" json:"hash_to"`
	Truncated bool   `json:"truncated"`
	Count     int    `json:"count"` // 去重合并次数

	At         time.Time `gorm:"index:idx_chg_at;index:idx_chg_kind_at,priority:2;index:idx_chg_key_at,priority:2" json:"at"`
	DetectedAt time.Time `json:"detected_at"`

	// Hash chain: PrevHash points at the previous event's ChainHash so that
	// deleting or rewriting a row breaks the chain and becomes detectable.
	PrevHash  string    `gorm:"size:64" json:"-"`
	ChainHash string    `gorm:"size:64" json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName keeps the table name stable and readable in sqlite.
func (ChangeEvent) TableName() string { return "change_events" }

// ChangeDiff holds the (already sanitized) unified diff body of one change.
// Bodies are kept for a shorter period than metadata and can be dropped
// entirely when the disk quota is hit — the metadata stays.
type ChangeDiff struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ChangeID  string    `gorm:"size:40;index" json:"change_id"`
	Seq       int       `json:"seq"`
	Body      string    `gorm:"type:text" json:"body"`
	Truncated bool      `json:"truncated"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName for ChangeDiff.
func (ChangeDiff) TableName() string { return "change_diffs" }

// ChangeOffset stores the read cursor of a log file (e.g. /var/log/dpkg.log) so
// that restarts do not re-parse the whole file, and rotation is detectable via
// inode change.
type ChangeOffset struct {
	Source    string    `gorm:"primaryKey;size:128" json:"source"`
	Inode     uint64    `json:"inode"`
	Offset    int64     `json:"offset"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName for ChangeOffset.
func (ChangeOffset) TableName() string { return "change_offsets" }

// ChangeIgnore is a user-editable glob that suppresses noisy paths.
// Hard-coded security blocks are NOT stored here — they live in code and cannot
// be removed through this table.
type ChangeIgnore struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Pattern   string    `gorm:"size:256;uniqueIndex" json:"pattern"`
	Note      string    `gorm:"size:256" json:"note"`
	CreatedAt time.Time `json:"created_at"`
}

// TableName for ChangeIgnore.
func (ChangeIgnore) TableName() string { return "change_ignore" }
