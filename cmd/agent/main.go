// OpsMini Agent main entrypoint.
package main

import (
	"flag"
	"fmt"
	"log"
	"time"

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
	// Security hardening: if the JWT secret is still the default weak value, generate a strong random key and write it back to the config.
	cfg.EnsureSecureSecret(*cfgPath)

	// Account recovery subcommand: operates on the database directly and exits without starting the server.
	if *resetMFA != "" {
		recoverAccount(cfg, *resetMFA, false)
		return
	}
	if *resetPass != "" {
		recoverAccount(cfg, *resetPass, true)
		return
	}

	db, err := store.Init(cfg.Database.Path)
	if err != nil {
		log.Fatalf("init store: %v", err)
	}

	// Metrics collection: samples every 10s, keeps 24 hours in memory (8640 points), and persists to SQLite (retained for 30 days).
	metrics := service.NewMetricCollector(10*time.Second, 8640, db)
	metrics.Start()
	defer metrics.Stop()

	r := router.New(cfg, db, metrics, version)
	log.Printf("OpsMini agent listening on %s", cfg.Server.Addr())
	if err := r.Run(cfg.Server.Addr()); err != nil {
		log.Fatalf("server: %v", err)
	}
}

// recoverAccount performs account recovery: resets MFA or password.
func recoverAccount(cfg *config.Config, username string, resetPassword bool) {
	db, err := store.Init(cfg.Database.Path)
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
