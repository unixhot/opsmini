// Package router assembles routes and dependency injection.
package router

import (
	"io/fs"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	v1 "github.com/opsmini/opsmini/internal/api/v1"
	"github.com/opsmini/opsmini/internal/agent"
	"github.com/opsmini/opsmini/internal/config"
	"github.com/opsmini/opsmini/internal/middleware"
	"github.com/opsmini/opsmini/internal/pkg/jwt"
	"github.com/opsmini/opsmini/internal/repository"
	"github.com/opsmini/opsmini/internal/service"
	"github.com/opsmini/opsmini/web"
)

// New constructs the Gin engine and registers routes. version is the build version, exposed via healthz for silent version detection by the frontend.
// agentMgr manages the Agent gRPC client lifecycle, allowing runtime reconfiguration of the OpsAnt link.
func New(cfg *config.Config, db *gorm.DB, metrics *service.MetricCollector, version string, agentMgr *agent.Manager) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()

	// Custom error pages (404 / 500) read the embedded error.html template and replace placeholders.
	errorHTML, _ := web.FS.ReadFile("error.html")
	serveErrorPage := func(c *gin.Context, status int, code, title, msg string) {
		html := string(errorHTML)
		if html == "" {
			html = "<!DOCTYPE html><html><head><meta charset='utf-8'><title>" + code + "</title></head><body style='font-family:sans-serif;text-align:center;padding:60px'><h1 style='font-size:64px;color:#4f6ef7'>" + code + "</h1><h2>" + title + "</h2><p style='color:#667'>" + msg + "</p><a href='/' style='color:#4f6ef7'>返回首页</a></body></html>"
		} else {
			html = strings.ReplaceAll(html, "__CODE__", code)
			html = strings.ReplaceAll(html, "__TITLE__", title)
			html = strings.ReplaceAll(html, "__MSG__", msg)
		}
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Data(status, "text/html; charset=utf-8", []byte(html))
	}
	r.Use(gin.CustomRecovery(func(c *gin.Context, err any) {
		log.Printf("panic recovered: %v", err)
		serveErrorPage(c, http.StatusInternalServerError, "500", "服务器内部错误", "服务器处理请求时发生异常，请稍后重试。")
	}), middleware.CORS())

	// Dependency injection
	userRepo := repository.NewUserRepo(db)
	settingSvc := service.NewSettingService(repository.NewSettingRepo(db))

	// JWT signing key is generated on first start and persisted in the database (not in the config file).
	jwtSecret, err := settingSvc.EnsureSecret("jwt_secret")
	if err != nil {
		log.Printf("warning: ensure jwt secret: %v", err)
	}
	jwtMgr := jwt.NewManager(
		jwtSecret,
		time.Duration(cfg.JWT.AccessTTL)*time.Second,
		time.Duration(cfg.JWT.RefreshTTL)*time.Second,
	)

	authSvc := service.NewAuthService(userRepo, jwtMgr, func() bool {
		v, err := settingSvc.Get("mfa_enabled")
		return err == nil && v == "true"
	}, func() bool {
		v, err := settingSvc.Get("allow_opsant_sso")
		// 默认打开：只有显式 "false" 才禁用（未设置/空/"true" 均为开启）
		return err != nil || v == "" || v == "true"
	}, cfg.Agent.ServerToken)
	userSvc := service.NewUserService(userRepo)
	auditSvc := service.NewAuditService(repository.NewAuditLogRepo(db))
	authHandler := v1.NewAuthHandler(authSvc, auditSvc)
	userHandler := v1.NewUserHandler(userSvc)
	roleSvc := service.NewRoleService(repository.NewRoleRepo(db))
	roleHandler := v1.NewRoleHandler(roleSvc)
	auditHandler := v1.NewAuditHandler(auditSvc)
	accessLogRepo := repository.NewAccessLogRepo(db)
	accessLogHandler := v1.NewAccessLogHandler(accessLogRepo)

	sysSvc := service.NewSystemService()
	dashboardHandler := v1.NewDashboardHandler(sysSvc, metrics)
	systemHandler := v1.NewSystemHandler(sysSvc)
	alertSvc := service.NewAlertService(repository.NewAlertRuleRepo(db))
	alertHandler := v1.NewAlertHandler(alertSvc)
	alertEventRepo := repository.NewAlertEventRepo(db)
	alertEventHandler := v1.NewAlertEventHandler(alertEventRepo)
	notificationRepo := repository.NewNotificationRepo(db)
	notificationSvc := service.NewNotificationService(notificationRepo)
	notificationHandler := v1.NewNotificationHandler(notificationSvc)
	mcpSvc := service.NewMcpService(repository.NewMcpRepo(db))
	mcpHandler := v1.NewMcpHandler(mcpSvc)

	websiteSvc := service.NewWebsiteService(repository.NewWebsiteRepo(db))
	websiteHandler := v1.NewWebsiteHandler(websiteSvc)
	databaseSvc := service.NewDatabaseService(repository.NewDatabaseRepo(db))
	databaseHandler := v1.NewDatabaseHandler(databaseSvc)
	cronSvc := service.NewCronService(repository.NewCronJobRepo(db))
	cronHandler := v1.NewCronHandler(cronSvc)
	crontabHandler := v1.NewCrontabHandler(service.NewCrontabService())
	fileSvc := service.NewFileService("/")
	fileHandler := v1.NewFileHandler(fileSvc)
	logSvc := service.NewLogService()
	logHandler := v1.NewLogHandler(logSvc)
	terminalHandler := v1.NewTerminalHandler(jwtMgr, roleSvc.HasPerm)

	// Host security module
	baselineSvc := service.NewBaselineService(repository.NewBaselineResultRepo(db), sysSvc)
	baselineHandler := v1.NewBaselineHandler(baselineSvc)
	loginSecSvc := service.NewLoginSecurityService()
	loginSecHandler := v1.NewLoginSecurityHandler(loginSecSvc)
	threatSvc := service.NewThreatService(repository.NewThreatFindingRepo(db))
	threatHandler := v1.NewThreatHandler(threatSvc)
	fimSvc := service.NewFimService(repository.NewFimBaselineRepo(db), repository.NewFimChangeRepo(db))
	fimHandler := v1.NewFimHandler(fimSvc)
	firewallSvc := service.NewFirewallService(sysSvc, cfg.Server.Port)
	firewallHandler := v1.NewFirewallHandler(firewallSvc)
	securitySvc := service.NewSecurityService(
		repository.NewBaselineResultRepo(db),
		repository.NewFimChangeRepo(db),
		repository.NewThreatFindingRepo(db),
		firewallSvc,
		loginSecSvc,
	)
	securityHandler := v1.NewSecurityHandler(securitySvc)
	securityMonitor := service.NewSecurityMonitor(fimSvc, threatSvc)

	// Docker service: if creation fails (e.g. misconfigured DOCKER_HOST), skip container routes and degrade gracefully.
	var (
		dockerHandler *v1.DockerHandler
		dockerSvc     *service.DockerService
	)
	if svc, err := service.NewDockerService(); err == nil {
		dockerSvc = svc
		dockerHandler = v1.NewDockerHandler(svc, jwtMgr, roleSvc.HasPerm)
	} else {
		log.Printf("docker client init failed, skipping container routes: %v", err)
	}

	settingHandler := v1.NewSettingHandler(settingSvc)

	// OpsAnt link configuration: panel settings override config.yaml, applied at runtime (Agent reconnect).
	agentConfigSvc := service.NewAgentConfigService(settingSvc, agentMgr, cfg.Agent, int32(cfg.Server.Port))
	agentConfigSvc.Init()
	agentConfigHandler := v1.NewAgentConfigHandler(agentConfigSvc)

	// Alert monitor: periodically evaluates rules, records events, and cleans up by retention time.
	alertMonitor := service.NewAlertMonitor(
		repository.NewAlertRuleRepo(db),
		alertEventRepo,
		notificationRepo,
		sysSvc,
		func() int {
			if v, err := settingSvc.Get("alert_retention_days"); err == nil && v != "" {
				if d, e := strconv.Atoi(v); e == nil && d > 0 {
					return d
				}
			}
			return 30
		},
		func() map[string]string {
			kv, _ := settingSvc.List()
			return kv
		},
	)
	alertMonitor.Start()
	securityMonitor.Start()

	// Log retention cleanup: hourly cleanup of expired audit logs and panel access logs (default 7 days).
	go func() {
		retention := func(key string, def int) int {
			if v, err := settingSvc.Get(key); err == nil && v != "" {
				if d, e := strconv.Atoi(v); e == nil && d > 0 {
					return d
				}
			}
			return def
		}
		ticker := time.NewTicker(time.Hour)
		for range ticker.C {
			if n, err := repository.NewAuditLogRepo(db).DeleteOlderThan(time.Now().AddDate(0, 0, -retention("audit_retention_days", 7))); err == nil && n > 0 {
				log.Printf("audit cleanup: removed %d logs", n)
			}
			if n, err := accessLogRepo.DeleteOlderThan(time.Now().AddDate(0, 0, -retention("access_retention_days", 7))); err == nil && n > 0 {
				log.Printf("access cleanup: removed %d logs", n)
			}
		}
	}()

	// AI skills: skill directory storage, integrating with SkillHub.
	skillSvc := service.NewSkillService(func() string {
		d, _ := settingSvc.Get("appstore_data_dir")
		return d
	})
	skillHandler := v1.NewSkillHandler(skillSvc)

	// AI assistant: runtime config is read from settings (overridable via the panel's AI tab); the OPSMINI_AI_KEY environment variable has the highest priority.
	// Built-in OpsMini MCP tools (system info / MCP list / Skill list / SkillHub search & install).
	aiSvc := service.NewAIService(&cfg.AI, func(key string) string {
		v, _ := settingSvc.Get(key)
		return v
	}, sysSvc, firewallSvc, mcpSvc, skillSvc)
	aiHandler := v1.NewAIHandler(aiSvc)

	// Software store: app list + one-click install/uninstall via Docker Shell scripts.
	appStoreSvc := service.NewAppStoreService(
		repository.NewAppRepo(db),
		dockerSvc,
		func() string {
			d, _ := settingSvc.Get("appstore_data_dir")
			return d
		},
	)
	appStoreHandler := v1.NewAppStoreHandler(appStoreSvc)
	appCategorySvc := service.NewAppCategoryService(repository.NewAppCategoryRepo(db))
	appCategoryHandler := v1.NewAppCategoryHandler(appCategorySvc)

	authMw := middleware.Auth(jwtMgr)
	// permMw performs fine-grained checks based on permission points.
	permMw := func(perm string) gin.HandlerFunc {
		return middleware.RequirePerm(roleSvc.HasPerm, perm)
	}
	loginLimiter := middleware.NewRateLimiter(10, time.Minute)

	// Secure entrypoint (optional prefix)
	root := r.Group("/")
	if cfg.Server.SecretEntry != "" {
		root = r.Group(cfg.Server.SecretEntry)
	}

	// Prometheus metrics (node_exporter compatible, for monitoring systems to scrape).
	// Basic-auth credentials priority: panel setting metrics_user/metrics_pass > config.yaml metrics.user/password; empty username means no authentication.
	metricsCreds := func() (string, string) {
		if u, err := settingSvc.Get("metrics_user"); err == nil && u != "" {
			p, _ := settingSvc.Get("metrics_pass")
			return u, p
		}
		return cfg.Metrics.User, cfg.Metrics.Password
	}
	if cfg.Metrics.Enabled {
		root.GET("/metrics", middleware.MetricsAuth(metricsCreds), systemHandler.Metrics)
	}

	api := root.Group("/api/v1")
	{
		// Public endpoints
		api.GET("/healthz", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"code": 0, "message": "ok", "data": gin.H{"status": "ok", "version": version}})
		})
		api.POST("/auth/login", loginLimiter.Limit(), authHandler.Login)
		api.POST("/auth/sso", authHandler.SSOLogin)
		api.POST("/auth/refresh", authHandler.Refresh)
		api.POST("/auth/mfa/verify", loginLimiter.Limit(), authHandler.VerifyMFA)
		api.POST("/auth/logout", authMw, authHandler.Logout)

		// Requires login
		authed := api.Group("")
		authed.Use(authMw, middleware.Audit(auditSvc), middleware.AccessLog(accessLogRepo))
		{
			// MFA binding management (requires login; audit is skipped for simplicity)
			authed.GET("/auth/mfa/status", authHandler.MFAStatus)
			authed.POST("/auth/mfa/setup", authHandler.MFASetup)
			authed.POST("/auth/mfa/enable", authHandler.MFAEnable)
			authed.POST("/auth/mfa/disable", authHandler.MFADisable)

			authed.GET("/users", userHandler.List)
			authed.POST("/users", permMw("user.create"), userHandler.Create)
			authed.PUT("/users/:id", permMw("user.edit"), userHandler.Update)
			authed.DELETE("/users/:id", permMw("user.delete"), userHandler.Delete)

			// Personal profile (current user)
			authed.GET("/profile", userHandler.Profile)
			authed.PUT("/profile", userHandler.UpdateProfile)
			authed.POST("/profile/password", userHandler.ChangePassword)

			// Roles & permissions (role CRUD is admin-only; permission groups / current permissions are readable by all logged-in users)
			authed.GET("/roles", roleHandler.List)
			authed.GET("/roles/groups", roleHandler.Groups)
			authed.GET("/permissions", roleHandler.MyPerms)
			authed.POST("/roles", permMw("user.create"), roleHandler.Create)
			authed.PUT("/roles/:id", permMw("user.edit"), roleHandler.Update)
			authed.DELETE("/roles/:id", permMw("user.delete"), roleHandler.Delete)

			// Audit logs / access logs (read-only info, open to all logged-in users)
			authed.GET("/audit-logs", auditHandler.List)
			authed.GET("/access-logs", accessLogHandler.List)

			// Host security
			authed.GET("/security/overview", securityHandler.Overview)
			authed.GET("/security/baseline/latest", baselineHandler.Latest)
			authed.POST("/security/baseline/scan", permMw("security.scan"), baselineHandler.Scan)
			authed.GET("/security/login", loginSecHandler.Analyze)
			authed.GET("/security/login/ssh", loginSecHandler.SSH)
			authed.GET("/security/firewall", firewallHandler.Status)
			authed.POST("/security/firewall/port", permMw("security.firewall"), firewallHandler.AllowPort)
			authed.DELETE("/security/firewall/port", permMw("security.firewall"), firewallHandler.DenyPort)
			authed.POST("/security/firewall/enable", permMw("security.firewall"), firewallHandler.Enable)
			authed.POST("/security/firewall/disable", permMw("security.firewall"), firewallHandler.Disable)
			authed.DELETE("/security/firewall/rule/:ref", permMw("security.firewall"), firewallHandler.DeleteRule)
			authed.GET("/security/fim/baselines", fimHandler.ListBaselines)
			authed.POST("/security/fim/rebuild", permMw("security.fim"), fimHandler.Rebuild)
			authed.DELETE("/security/fim/baselines/:id", permMw("security.fim"), fimHandler.RemoveBaseline)
			authed.GET("/security/fim/events", fimHandler.ListChanges)
			authed.GET("/security/threats", threatHandler.List)
			authed.POST("/security/threats/scan", permMw("security.threat"), threatHandler.Scan)
			authed.POST("/security/threats/:id/resolve", permMw("security.threat"), threatHandler.Resolve)

			// Panel settings (runtime configuration persisted)
			authed.GET("/settings", permMw("settings.view"), settingHandler.List)
			authed.PUT("/settings", permMw("settings.edit"), settingHandler.Update)

			// OpsAnt link configuration (panel visualization)
			authed.GET("/agent-config", permMw("settings.view"), agentConfigHandler.Get)
			authed.PUT("/agent-config", permMw("settings.edit"), agentConfigHandler.Update)

			// Dashboard / monitoring
			authed.GET("/dashboard/overview", dashboardHandler.Overview)
			authed.GET("/dashboard/metrics", dashboardHandler.Metrics)

			// System management
			authed.GET("/system/info", systemHandler.Info)
			authed.GET("/system/monitor", systemHandler.MonitorSummary)
			authed.GET("/system/processes", systemHandler.Processes)
			authed.POST("/system/process/kill", permMw("system.process"), systemHandler.Kill)
			authed.GET("/system/ports", systemHandler.Ports)
			authed.GET("/system/disks", systemHandler.Disks)
			authed.GET("/system/network", systemHandler.Networks)
			authed.GET("/system/routes", systemHandler.Routes)
			authed.GET("/system/users", systemHandler.Users)
			authed.GET("/system/groups", systemHandler.Groups)
			authed.GET("/system/firewall", systemHandler.Firewall)

			// AI assistant + MCP
			authed.POST("/ai/chat", permMw("ai.use"), aiHandler.Chat)
			authed.POST("/ai/chat/stream", permMw("ai.use"), aiHandler.ChatStream)
			authed.GET("/mcp", mcpHandler.List)
			authed.POST("/mcp", permMw("mcp.manage"), mcpHandler.Create)
			authed.PUT("/mcp/:id", permMw("mcp.manage"), mcpHandler.Update)
			authed.DELETE("/mcp/:id", permMw("mcp.manage"), mcpHandler.Delete)

			// AI skills
			authed.GET("/skills", permMw("skill.view"), skillHandler.List)
			authed.GET("/skills/catalog", permMw("skill.view"), skillHandler.Catalog)
			authed.GET("/skills/search", permMw("skill.view"), skillHandler.Search)
			authed.POST("/skills", permMw("skill.manage"), skillHandler.Create)
			authed.POST("/skills/install", permMw("skill.manage"), skillHandler.Install)
			authed.POST("/skills/upload", permMw("skill.manage"), skillHandler.Upload)
			authed.DELETE("/skills/:name", permMw("skill.manage"), skillHandler.Delete)
			authed.POST("/skills/:name/toggle", permMw("skill.manage"), skillHandler.Toggle)

			// Alert rules
			authed.GET("/alert-rules", alertHandler.List)
			authed.POST("/alert-rules", permMw("alert.manage"), alertHandler.Create)
			authed.PUT("/alert-rules/:id", permMw("alert.manage"), alertHandler.Update)
			authed.DELETE("/alert-rules/:id", permMw("alert.manage"), alertHandler.Delete)

			// Alert events
			authed.GET("/alert-events", alertEventHandler.List)
			authed.GET("/alert-events/active", alertEventHandler.Active)
			authed.DELETE("/alert-events/:id", permMw("alert.manage"), alertEventHandler.Delete)

			// Notifications
			authed.GET("/notifications", notificationHandler.List)
			authed.GET("/notifications/unread-count", notificationHandler.UnreadCount)
			authed.PUT("/notifications/read-all", notificationHandler.MarkAllRead)
			authed.PUT("/notifications/:id/read", notificationHandler.MarkRead)
			authed.DELETE("/notifications", notificationHandler.ClearAll)
			authed.DELETE("/notifications/:id", notificationHandler.Delete)

			// Website management
			authed.GET("/websites", websiteHandler.List)
			authed.POST("/websites", permMw("website.create"), websiteHandler.Create)
			authed.PUT("/websites/:id", permMw("website.edit"), websiteHandler.Update)
			authed.DELETE("/websites/:id", permMw("website.delete"), websiteHandler.Delete)

			// App categories
			authed.GET("/app-categories", appCategoryHandler.List)
			authed.POST("/app-categories", permMw("apps.install"), appCategoryHandler.Create)
			authed.PUT("/app-categories/:id", permMw("apps.install"), appCategoryHandler.Update)
			authed.DELETE("/app-categories/:id", permMw("apps.install"), appCategoryHandler.Delete)

			// App templates (software store)
			authed.GET("/apps", appStoreHandler.List)
			authed.GET("/apps/:slug", appStoreHandler.Get)
			authed.POST("/apps", permMw("apps.install"), appStoreHandler.Create)
			authed.POST("/apps/import", permMw("apps.install"), appStoreHandler.Import)
			authed.POST("/apps/sync-official", permMw("apps.install"), appStoreHandler.SyncOfficial)
			authed.PUT("/apps/:slug", permMw("apps.install"), appStoreHandler.Update)
			authed.DELETE("/apps/:slug", permMw("apps.install"), appStoreHandler.Delete)
			authed.GET("/apps/:slug/export", permMw("apps.install"), appStoreHandler.Export)
			authed.GET("/apps/:slug/logo", appStoreHandler.Logo)
			authed.POST("/apps/:slug/install", permMw("apps.install"), appStoreHandler.Install)
			authed.POST("/apps/:slug/uninstall", permMw("apps.install"), appStoreHandler.Uninstall)
			authed.POST("/apps/:slug/start", permMw("apps.install"), appStoreHandler.Start)
			authed.POST("/apps/:slug/stop", permMw("apps.install"), appStoreHandler.Stop)
			authed.POST("/apps/:slug/restart", permMw("apps.install"), appStoreHandler.Restart)

			// Database management
			authed.GET("/databases", databaseHandler.List)
			authed.POST("/databases", permMw("database.create"), databaseHandler.Create)
			authed.PUT("/databases/:id", permMw("database.edit"), databaseHandler.Update)
			authed.DELETE("/databases/:id", permMw("database.delete"), databaseHandler.Delete)

			// Scheduled tasks
			authed.GET("/cron-jobs", cronHandler.List)
			authed.POST("/cron-jobs", permMw("cron.create"), cronHandler.Create)
			authed.PUT("/cron-jobs/:id", permMw("cron.edit"), cronHandler.Update)
			authed.DELETE("/cron-jobs/:id", permMw("cron.delete"), cronHandler.Delete)
			authed.GET("/cron-jobs/system", crontabHandler.SystemJobs)
			authed.GET("/cron-jobs/user", crontabHandler.UserJobs)

			// File management
			authed.GET("/files", fileHandler.List)
			authed.GET("/files/download", permMw("file.view"), fileHandler.Download)
			authed.GET("/files/download-dir", permMw("file.view"), fileHandler.DownloadDir)
			authed.GET("/files/du", permMw("file.view"), fileHandler.Du)
			authed.POST("/files/mkdir", permMw("file.write"), fileHandler.MakeDir)
			authed.POST("/files/rename", permMw("file.write"), fileHandler.Rename)
			authed.POST("/files/delete", permMw("file.delete"), fileHandler.Delete)
			authed.POST("/files/upload", permMw("file.write"), fileHandler.Upload)

			// Logs
			authed.GET("/logs", logHandler.List)
			authed.GET("/logs/tail", logHandler.Tail)

			// Container management (Docker)
			if dockerHandler != nil {
				authed.GET("/containers", dockerHandler.Containers)
				authed.GET("/containers/:id", dockerHandler.Inspect)
				authed.GET("/containers/:id/logs", dockerHandler.Logs)
				authed.POST("/containers/:id/start", permMw("container.edit"), dockerHandler.StartContainer)
				authed.POST("/containers/:id/stop", permMw("container.edit"), dockerHandler.StopContainer)
				authed.POST("/containers/:id/restart", permMw("container.edit"), dockerHandler.RestartContainer)
				authed.DELETE("/containers/:id", permMw("container.delete"), dockerHandler.RemoveContainer)
				authed.GET("/images", dockerHandler.Images)
				authed.POST("/images/pull", permMw("container.edit"), dockerHandler.PullImage)
				authed.DELETE("/images/:id", permMw("container.delete"), dockerHandler.RemoveImage)
				authed.GET("/volumes", dockerHandler.Volumes)
				authed.POST("/volumes", permMw("container.edit"), dockerHandler.CreateVolume)
				authed.DELETE("/volumes/:id", permMw("container.delete"), dockerHandler.RemoveVolume)
				authed.GET("/networks", dockerHandler.Networks)
				authed.POST("/networks", permMw("container.edit"), dockerHandler.CreateNetwork)
				authed.DELETE("/networks/:id", permMw("container.delete"), dockerHandler.RemoveNetwork)
			}
		}

		// Web terminal (WebSocket, query token authentication, bypassing the HTTP header auth middleware)
		api.GET("/terminal", terminalHandler.Serve)

		// Container exec terminal (WebSocket, query token authentication, same protocol as the host terminal)
		if dockerHandler != nil {
			api.GET("/containers/:id/exec", dockerHandler.ContainerExec)
		}
	}

	// Frontend static assets (embedded in the single binary). Static library content is stable, so add a one-year strong cache to avoid re-downloading large libraries on every login.
	staticFS, err := fs.Sub(web.FS, "static")
	if err != nil {
		log.Printf("embed static sub: %v", err)
	} else {
		fileServer := http.FileServer(http.FS(staticFS))
		r.GET("/static/*filepath", func(c *gin.Context) {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
			http.StripPrefix("/static", fileServer).ServeHTTP(c.Writer, c.Request)
		})
	}
	indexHTML, err := web.FS.ReadFile("index.html")
	if err != nil {
		log.Fatalf("embed index.html: %v", err)
	}
	serveIndex := func(c *gin.Context) {
		// Disable caching of index.html to avoid the browser still using the old frontend after a version upgrade (requires a manual refresh).
		c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
		c.Header("Pragma", "no-cache")
		c.Header("Expires", "0")
		c.Data(http.StatusOK, "text/html; charset=utf-8", indexHTML)
	}
	r.GET("/", serveIndex)
	// SPA fallback: the root path returns index.html; API paths return a JSON 404; other unknown paths return the custom 404 page.
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		if strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/agent/") {
			c.JSON(http.StatusNotFound, gin.H{"code": 40401, "message": "not found"})
			return
		}
		if strings.HasPrefix(p, "/static/") {
			serveErrorPage(c, http.StatusNotFound, "404", "资源不存在", "您访问的静态资源不存在或已被移除。")
			return
		}
		if p != "/" {
			serveErrorPage(c, http.StatusNotFound, "404", "页面不存在", "您访问的页面不存在或已被移除。")
			return
		}
		serveIndex(c)
	})
	return r
}
