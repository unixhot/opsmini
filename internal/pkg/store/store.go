// Package store handles SQLite initialization, migration, and seed data.
package store

import (
	"crypto/rand"
	"io"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/opsmini/opsmini/internal/model"
)

// Init opens SQLite (pure Go driver), auto-migrates, and writes seed data.
// logLevel controls the GORM logger verbosity (info/warn/error); logWriter is where logs are written.
func Init(path string, logLevel string, logWriter io.Writer) (*gorm.DB, error) {
	gormLogger := logger.New(
		log.New(logWriter, "", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  gormLogLevel(logLevel),
			IgnoreRecordNotFoundError: true, // "record not found" is normal control flow, not an error
			Colorful:                  false,
		},
	)
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: gormLogger})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(&model.User{}, &model.Website{}, &model.Database{}, &model.CronJob{}, &model.AuditLog{}, &model.Setting{}, &model.App{}, &model.AlertRule{}, &model.McpServer{}, &model.AlertEvent{}, &model.AccessLog{}, &model.BaselineResult{}, &model.FimBaseline{}, &model.FimChange{}, &model.ThreatFinding{}, &model.Role{}, &model.AppCategory{}, &model.Notification{}, &model.MetricPoint{}, &model.AISession{}, &model.AIMessage{}, &model.AIPendingApproval{}, &model.Signal{}, &model.Incident{}, &model.TriggerPolicy{}, &model.AgentRun{}, &model.ActionLog{}, &model.QuarantineItem{}, &model.SnapshotRecord{}, &model.Baseline{}, &model.SignalFeedback{}, &model.BlockedIP{}, &model.ChangeEvent{}, &model.ChangeDiff{}, &model.ChangeOffset{}, &model.ChangeIgnore{}, &model.ChangeChainMeta{}, &model.Campaign{}, &model.ActionJournal{}, &model.ShadowReview{}, &model.ChangeReview{}); err != nil {
		return nil, err
	}
	if err := seedAdmin(db); err != nil {
		return nil, err
	}
	if err := seedRoles(db); err != nil {
		return nil, err
	}
	if err := seedCategories(db); err != nil {
		return nil, err
	}
	if err := seedApps(db); err != nil {
		return nil, err
	}
	if err := seedAlertRules(db); err != nil {
		return nil, err
	}
	return db, nil
}

// gormLogLevel maps a config log level string to a GORM logger level.
func gormLogLevel(level string) logger.LogLevel {
	switch strings.ToLower(level) {
	case "info":
		return logger.Info
	case "warn":
		return logger.Warn
	default: // "error" or empty
		return logger.Error
	}
}

// seedCategories writes the four built-in categories (websites/databases/middleware/others).
func seedCategories(db *gorm.DB) error {
	for _, builtin := range model.BuiltinCategories() {
		var count int64
		if err := db.Model(&model.AppCategory{}).Where("key = ?", builtin.Key).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := db.Create(&builtin).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// seedRoles writes the three built-in roles (creates them if absent).
func seedRoles(db *gorm.DB) error {
	for _, builtin := range model.BuiltinRoles() {
		var count int64
		if err := db.Model(&model.Role{}).Where("name = ?", builtin.Name).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := db.Create(&builtin).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// seedAlertRules writes default alert rules (idempotent: inserts a built-in rule only if its name does not exist).
// Covers node_exporter core metrics so the panel ships with practical, community-standard alert rules out of the box.
func seedAlertRules(db *gorm.DB) error {
	rules := []model.AlertRule{
		{Name: "CPU 过高", Metric: "CPU 使用率", Condition: "> 90%", Duration: "5 分钟", Notify: "面板,邮件,企业微信,钉钉,飞书", Enabled: true},
		{Name: "内存不足", Metric: "内存使用率", Condition: "> 85%", Duration: "5 分钟", Notify: "面板,邮件", Enabled: true},
		{Name: "磁盘即将写满", Metric: "磁盘使用率", Condition: "> 90%", Duration: "1 分钟", Notify: "面板,邮件,企业微信,钉钉,飞书", Enabled: true},
		{Name: "磁盘 inode 即将耗尽", Metric: "磁盘 inode 使用率", Condition: "> 90%", Duration: "5 分钟", Notify: "面板,邮件", Enabled: true},
		{Name: "系统负载过高", Metric: "系统负载(5分钟)", Condition: "> CPU 核数", Duration: "5 分钟", Notify: "面板,邮件", Enabled: true},
		{Name: "Swap 使用率过高", Metric: "Swap 使用率", Condition: "> 80%", Duration: "5 分钟", Notify: "面板,邮件", Enabled: true},
		{Name: "磁盘 IO 繁忙", Metric: "磁盘 IO 使用率", Condition: "> 90%", Duration: "5 分钟", Notify: "面板", Enabled: true},
		{Name: "网络接收错误", Metric: "网卡接收错误", Condition: "> 0", Duration: "5 分钟", Notify: "面板", Enabled: true},
		{Name: "TCP 连接数过高", Metric: "TCP 当前连接数", Condition: "> 1000", Duration: "5 分钟", Notify: "面板", Enabled: true},
		{Name: "CPU 温度过高", Metric: "CPU 温度", Condition: "> 80°C", Duration: "5 分钟", Notify: "面板,邮件", Enabled: true},
		{Name: "运行中进程数过多", Metric: "运行中进程数", Condition: "> 500", Duration: "5 分钟", Notify: "面板", Enabled: true},
		{Name: "上下文切换过高", Metric: "上下文切换频率", Condition: "> 100000", Duration: "5 分钟", Notify: "面板", Enabled: true},
		{Name: "僵尸进程堆积", Metric: "僵尸进程数", Condition: "> 5", Duration: "5 分钟", Notify: "面板,邮件", Enabled: true},
		{Name: "系统刚刚重启", Metric: "系统运行时长", Condition: "< 300", Duration: "0 分钟", Notify: "面板,邮件,企业微信", Enabled: true},

		// Event-type rules. These have no sampled value: the metric is "how many
		// happened inside a window", so the same threshold engine applies and the
		// alert clears by itself once the window slides past the last occurrence.
		{Name: "系统时钟偏移过大", Metric: "NTP 时间偏移", Condition: "> 5", Duration: "5 分钟", Notify: "面板,邮件", Enabled: true},
		{Name: "文件系统变为只读", Metric: "只读文件系统数", Condition: "> 0", Duration: "1 分钟", Notify: "面板,邮件,企业微信", Enabled: true},
		{Name: "文件系统错误", Metric: "文件系统错误数", Condition: "> 0", Duration: "1 分钟", Notify: "面板,邮件,企业微信,钉钉,飞书", Enabled: true},
		{Name: "OOM Killer 触发", Metric: "OOM 事件数", Condition: "> 0", Duration: "1 分钟", Notify: "面板,邮件,企业微信", Enabled: true},
		{Name: "硬件或内核错误", Metric: "硬件错误数", Condition: "> 0", Duration: "1 分钟", Notify: "面板,邮件", Enabled: true},
		{Name: "Docker 服务不可用", Metric: "Docker 服务不可用", Condition: "> 0", Duration: "5 分钟", Notify: "面板,邮件,企业微信", Enabled: true},
		{Name: "容器异常退出", Metric: "容器异常退出数", Condition: "> 0", Duration: "1 分钟", Notify: "面板", Enabled: true},
		{Name: "计划任务执行失败", Metric: "计划任务失败数", Condition: "> 0", Duration: "5 分钟", Notify: "面板,邮件", Enabled: true},
		{Name: "备份任务失败", Metric: "备份失败数", Condition: "> 0", Duration: "5 分钟", Notify: "面板,邮件,企业微信", Enabled: true},
	}
	// 指标口径修正：/proc/stat 的 ctxt 是开机以来的累计值，按「次」比阈值会永久
	// 告警；已部署的库里这条规则仍叫旧名字，这里就地改名，不新插一条。
	db.Model(&model.AlertRule{}).Where("metric = ?", "上下文切换次数").Update("metric", "上下文切换频率")

	// v1.4.3 移除了内置日志采集：内置规则不再插入，但老库里那条和它挂着的
	// active 事件必须就地清掉，否则规则永远停在「未知」、事件永远不 resolve。
	// 幂等：没有匹配行时是空操作。
	db.Where("name = ?", "日志管道积压").Delete(&model.AlertRule{})
	db.Model(&model.AlertEvent{}).
		Where("metric = ? AND status = ?", "日志管道积压", "active").
		Updates(map[string]any{"status": "resolved", "resolved_at": time.Now()})

	inserted := 0
	for _, r := range rules {
		var count int64
		if err := db.Model(&model.AlertRule{}).Where("name = ?", r.Name).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			if err := db.Create(&r).Error; err != nil {
				return err
			}
			inserted++
		}
	}
	if inserted > 0 {
		log.Printf("seeded %d alert rules", inserted)
	}
	return nil
}

// seedApps writes default software store apps on first run (when the app table is empty).
func seedApps(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.App{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	apps := seedAppList()
	for i := range apps {
		if err := db.Create(&apps[i]).Error; err != nil {
			return err
		}
	}
	log.Printf("seeded %d app store apps", len(apps))
	return nil
}

// seedAdmin ensures the default admin opsmini exists. On first install it generates a random strong password,
// and outputs it via the log (only once); afterwards the password must be changed in the panel.
func seedAdmin(db *gorm.DB) error {
	var count int64
	if err := db.Model(&model.User{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	// Initial password priority: environment variable OPSMINI_INIT_PASSWORD > random generation.
	// The install script (install/install.sh) injects a preset password via this variable,
	// so the password can be written to the .init_passwd file and shown to the user.
	pwd := os.Getenv("OPSMINI_INIT_PASSWORD")
	generated := pwd == ""
	if generated {
		var err error
		pwd, err = generatePassword(16)
		if err != nil {
			return err
		}
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := db.Create(&model.User{
		Username:     "opsmini",
		PasswordHash: string(hash),
		Role:         model.RoleAdmin,
		Status:       1,
	}).Error; err != nil {
		return err
	}
	if generated {
		// Write the random password to .init_passwd (0600) to avoid plaintext ending up in stdout/systemd logs.
		if werr := os.WriteFile(".init_passwd", []byte(pwd+"\n"), 0o600); werr != nil {
			log.Printf("initial admin account created: username=opsmini，密码写入失败，请用 -reset-pass 重置")
		} else {
			log.Printf("initial admin account created: username=opsmini，初始密码已写入 .init_passwd")
		}
	} else {
		log.Printf("initial admin account created: username=opsmini（密码来自 OPSMINI_INIT_PASSWORD）")
	}
	return nil
}

// generatePassword generates an n-character password using cryptographically secure randomness.
// The character set removes easily confused characters (0/O, 1/l/I, etc.).
func generatePassword(n int) (string, error) {
	const chars = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"
	b := make([]byte, n)
	for i := range b {
		idx, err := rand.Int(rand.Reader, big.NewInt(int64(len(chars))))
		if err != nil {
			return "", err
		}
		b[i] = chars[idx.Int64()]
	}
	return string(b), nil
}

// ResetUserMFA resets the MFA binding for the specified user (for recovery after losing the authenticator code).
func ResetUserMFA(db *gorm.DB, username string) error {
	var u model.User
	if err := db.Where("username = ?", username).First(&u).Error; err != nil {
		return err
	}
	u.MFAEnabled = false
	u.MFASecret = ""
	return db.Save(&u).Error
}

// ResetUserPassword resets the specified user's password to a random strong password and returns the plaintext.
func ResetUserPassword(db *gorm.DB, username string) (string, error) {
	var u model.User
	if err := db.Where("username = ?", username).First(&u).Error; err != nil {
		return "", err
	}
	pwd, err := generatePassword(16)
	if err != nil {
		return "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	u.PasswordHash = string(hash)
	return pwd, db.Save(&u).Error
}
