// OpsMini Agent main entrypoint.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"github.com/opsmini/opsmini/internal/agent"
	"github.com/opsmini/opsmini/internal/config"
	"github.com/opsmini/opsmini/internal/pkg/store"
	"github.com/opsmini/opsmini/internal/router"
	"github.com/opsmini/opsmini/internal/service"
)

// Build info, injected via -ldflags "-X main.version=..." (see Makefile).
var (
	version   = "dev"
	buildTime = "unknown"
	gitCommit = "unknown"
)

func main() {
	cfgPath := flag.String("config", "configs/config.yaml", "配置文件路径")
	showVersion := flag.Bool("version", false, "打印版本信息并退出")
	resetMFA := flag.String("reset-mfa", "", "重置指定用户的 MFA 绑定（丢失验证码时使用）")
	resetPass := flag.String("reset-pass", "", "重置指定用户密码为随机强密码并打印")
	flag.Parse()

	if *showVersion {
		fmt.Printf("opsmini %s (commit %s, built %s)\n", version, gitCommit, buildTime)
		return
	}

	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	// Note: the JWT signing key and Agent API token are generated on first start and persisted
	// in the database (see router.New -> SettingService.EnsureSecret), not in the config file.

	// Configure logging: writes to the configured log file (or stdout) and sets the log level.
	logWriter, err := initLogging(cfg)
	if err != nil {
		log.Printf("warning: init logging: %v (falling back to stdout)", err)
		logWriter = os.Stdout
	}

	// Account recovery subcommand: operates on the database directly and exits without starting the server.
	if *resetMFA != "" {
		recoverAccount(cfg, *resetMFA, false)
		return
	}
	if *resetPass != "" {
		recoverAccount(cfg, *resetPass, true)
		return
	}

	db, err := store.Init(cfg.Database.Path, cfg.Log.Level, logWriter)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}

	// Metrics collection: samples every 10s, keeps 24 hours in memory (8640 points), and persists to SQLite (retained for 30 days).
	metrics := service.NewMetricCollector(10*time.Second, 8640, db)
	metrics.Start()
	defer metrics.Stop()

	// gRPC Agent lifecycle: managed by the router so the panel can reconnect it at
	// runtime (empty server_addr means disconnected; config.yaml is the fallback).
	hostname, _ := os.Hostname()
	agentMgr := agent.NewManager(hostname, version)

	r := router.New(cfg, db, metrics, version, agentMgr)
	log.Printf("OpsMini agent listening on %s", cfg.Server.Addr())
	if err := r.Run(cfg.Server.Addr()); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// initLogging configures the standard logger output and returns the writer used for logs.
// When cfg.Log.Path is set, logs are written to that file (and stdout); otherwise they go to stdout only.
func initLogging(cfg *config.Config) (io.Writer, error) {
	if cfg.Log.Path == "" {
		return os.Stdout, nil
	}
	f, err := os.OpenFile(cfg.Log.Path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return os.Stdout, err
	}
	// Write to both the file and stdout so systemd journal keeps a copy while a file exists for easy reading.
	w := io.MultiWriter(os.Stdout, f)
	log.SetOutput(w)
	return w, nil
}

// recoverAccount performs account recovery: resets MFA or password.
func recoverAccount(cfg *config.Config, username string, resetPassword bool) {
	db, err := store.Init(cfg.Database.Path, cfg.Log.Level, os.Stdout)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}
	if resetPassword {
		pwd, err := store.ResetUserPassword(db, username)
		if err != nil {
			log.Fatalf("重置密码失败: %v", err)
		}
		fmt.Printf("已重置用户 %s 的密码为：%s\n", username, pwd)
		fmt.Println("请立即使用新密码登录，并在面板中修改密码。")
	} else {
		if err := store.ResetUserMFA(db, username); err != nil {
			log.Fatalf("重置 MFA 失败: %v", err)
		}
		fmt.Printf("已重置用户 %s 的双因素验证（MFA），可重新登录并在面板中重新绑定。\n", username)
	}
}
