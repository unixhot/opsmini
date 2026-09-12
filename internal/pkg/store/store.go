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
	if err := db.AutoMigrate(&model.User{}, &model.Website{}, &model.Database{}, &model.CronJob{}, &model.AuditLog{}, &model.Setting{}, &model.App{}, &model.AlertRule{}, &model.McpServer{}, &model.AlertEvent{}, &model.AccessLog{}, &model.BaselineResult{}, &model.FimBaseline{}, &model.FimChange{}, &model.ThreatFinding{}, &model.Role{}, &model.AppCategory{}, &model.Notification{}, &model.MetricPoint{}); err != nil {
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
		{Name: "上下文切换过高", Metric: "上下文切换次数", Condition: "> 100000", Duration: "5 分钟", Notify: "面板", Enabled: true},
	}
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
