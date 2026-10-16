package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/agent"
	"github.com/opsmini/opsmini/internal/aiagent"
	"github.com/opsmini/opsmini/internal/aiagent/tools"
	v1 "github.com/opsmini/opsmini/internal/api/v1"
	"github.com/opsmini/opsmini/internal/config"
	"github.com/opsmini/opsmini/internal/repository"
	"github.com/opsmini/opsmini/internal/service"
)

// toolDeps groups the services the built-in agent tools operate on.
type toolDeps struct {
	sys         *service.SystemService
	firewall    *service.FirewallService
	file        *service.FileService
	log         *service.LogService
	cron        *service.CronService
	crontab     *service.CrontabService
	website     *service.WebsiteService
	database    *service.DatabaseService
	appStore    *service.AppStoreService
	docker      *service.DockerService
	security    *service.SecurityService
	skill       *service.SkillService
	mcp         *service.McpService
	metrics     *service.MetricCollector
	agentMgr    *agent.Manager
	agentConfig *service.AgentConfigService
}

// settingKeyToolPolicy is the panel setting that stores the per-tool policy
// overrides as a JSON object ({"exec_command":"confirm", ...}).
const settingKeyToolPolicy = "ai_tool_policy"

// aiAgentBundle is what buildAIAgent hands back to the router.
type aiAgentBundle struct {
	handler   *v1.AgentHandler
	mcpServer *aiagent.MCPServer
}

// buildAIAgent assembles the AI Agent: LLM client, tool registry, guard rails,
// conversation store, approval store, MCP client (outbound) and MCP server
// (inbound).
func buildAIAgent(
	cfg *config.Config,
	db *gorm.DB,
	settingSvc *service.SettingService,
	auditSvc *service.AuditService,
	roleSvc *service.RoleService,
	userSvc *service.UserService,
	agentMgr *agent.Manager,
	version string,
	d toolDeps,
) aiAgentBundle {
	getSetting := func(key string) string {
		v, _ := settingSvc.Get(key)
		return v
	}
	dataDir := func() string {
		if v := getSetting("appstore_data_dir"); v != "" {
			return v
		}
		return "/var/lib/opsmini/data"
	}

	// Resolve the model configuration on every call so the panel's AI settings
	// (and the OPSMINI_AI_KEY environment variable) take effect immediately.
	llm := aiagent.NewLLMClient(func() aiagent.LLMConfig {
		base := getSetting("ai_base_url")
		if base == "" {
			base = cfg.AI.BaseURL
		}
		key := os.Getenv("OPSMINI_AI_KEY")
		if key == "" {
			key = getSetting("ai_api_key")
		}
		if key == "" {
			key = cfg.AI.APIKey
		}
		model := getSetting("ai_model")
		if model == "" {
			model = cfg.AI.Model
		}
		return aiagent.LLMConfig{BaseURL: base, APIKey: key, Model: model}
	})

	repo := repository.NewAIRepo(db)

	// Memory compression: fold old turns into a short digest with the model.
	summarize := func(prompt string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		msg, _, err := llm.Chat(ctx, []aiagent.Message{
			{Role: "system", Content: "你是运维对话摘要助手。请把下面的对话压缩为要点，保留：用户诉求、已确认的结论、执行过的关键操作与其结果、发现的问题、未完成的待办。控制在 300 字以内，直接输出摘要正文。"},
			{Role: "user", Content: prompt},
		}, nil)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(msg.Content), nil
	}

	store := aiagent.NewStore(repo, summarize)
	approver := aiagent.NewApprover(repo, 10*time.Minute)

	// Tool registry.
	reg := aiagent.NewRegistry()

	// Remote control plane (M8-E): derived from the Agent configuration so the
	// panel can drive every host enrolled on the OpsMini Server. The HTTP control
	// plane defaults to the gRPC host with port 4508; both can be overridden.
	remoteClient := tools.NewRemoteClient(
		func() string {
			if v := getSetting("ai_remote_server_url"); v != "" {
				return v
			}
			addr, _, _ := d.agentConfig.Current()
			host := addr
			if i := strings.LastIndex(addr, ":"); i > 0 {
				host = addr[:i]
			}
			if host == "" {
				return ""
			}
			return "http://" + host + ":4508"
		},
		func() string {
			if v := getSetting("ai_remote_server_token"); v != "" {
				return v
			}
			_, token, _ := d.agentConfig.Current()
			return token
		},
		dataDir,
	)

	tools.Build(reg, tools.Deps{
		Sys:       d.sys,
		Firewall:  d.firewall,
		File:      d.file,
		Log:       d.log,
		Cron:      d.cron,
		Crontab:   d.crontab,
		Website:   d.website,
		Database:  d.database,
		AppStore:  d.appStore,
		Docker:    d.docker,
		Security:  d.security,
		Skill:     d.skill,
		Mcp:       d.mcp,
		Metrics:   d.metrics,
		Agent:     agentMgr,
		Remote:    remoteClient,
		DataDir:   dataDir,
		PanelPort: cfg.Server.Port,
		Setting:   getSetting,
	})

	guard := aiagent.NewGuard(dataDir, cfg.Server.Port)

	// MCP client: connect to the MCP servers configured in the panel and expose
	// their tools to the agent (M8-C). Connections are reconciled periodically so
	// configuration changes take effect without a restart.
	mcpMgr := aiagent.NewMCPClientManager(reg, version)
	reloadMCP := func() {
		list, err := d.mcp.List()
		if err != nil {
			return
		}
		mcpMgr.Reload(list)
	}
	go func() {
		// Wait for the HTTP listener before the first attempt: an MCP server that
		// points back at OpsMini itself is only reachable once the server is up.
		time.Sleep(3 * time.Second)
		reloadMCP()
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			reloadMCP()
		}
	}()

	policies := func() map[string]string {
		out := map[string]string{}
		raw := getSetting(settingKeyToolPolicy)
		if raw == "" {
			return out
		}
		_ = json.Unmarshal([]byte(raw), &out)
		return out
	}
	savePolicies := func(m map[string]string) error {
		b, err := json.Marshal(m)
		if err != nil {
			return err
		}
		return settingSvc.Set(map[string]string{settingKeyToolPolicy: string(b)})
	}

	a := aiagent.NewAgent(llm, reg, guard, store, approver, aiagent.Config{
		MaxSteps:    12,
		StepTimeout: 120 * time.Second,
		Snapshot: func() string {
			return agentSnapshot(d.sys)
		},
	})
	a.SetAuditor(auditSvc.Record)
	a.SetPolicyResolver(func(name string) string { return policies()[name] })
	a.SetPermissionChecker(roleSvc.HasPerm)
	a.SetCompressor(func(sessionID string) {
		if err := store.Compress(sessionID); err != nil {
			log.Printf("ai agent: compress session %s: %v", sessionID, err)
		}
	})

	// Expire stale approvals in the background.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			if n := approver.Sweep(); n > 0 {
				log.Printf("ai agent: expired %d pending approvals", n)
			}
		}
	}()

	// Clean up old conversations (retention configurable, default 30 days).
	go func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			days := 30
			if v := getSetting("ai_session_retention_days"); v != "" {
				if n, err := time.ParseDuration(v + "h"); err == nil {
					days = int(n.Hours() / 24)
				}
			}
			if days <= 0 {
				days = 30
			}
			if n, err := repo.DeleteSessionsBefore(time.Now().AddDate(0, 0, -days)); err == nil && n > 0 {
				log.Printf("ai agent: cleaned up %d old sessions", n)
			}
		}
	}()

	// Inbound MCP server (M8-F): publish the same tool set over the Model Context
	// Protocol so external clients (Claude Desktop, Cursor, OpsAnt, another
	// OpsMini) can use it. Protected by a bearer token generated on first start.
	mcpServer := aiagent.NewMCPServer(reg, guard, version)
	mcpServer.Token = func() string {
		if v := getSetting("mcp_server_token"); v != "" {
			return v
		}
		// EnsureSecret generates and persists a token on first use.
		t, err := settingSvc.EnsureSecret("mcp_server_token")
		if err != nil {
			log.Printf("ai agent: ensure mcp server token: %v", err)
			return ""
		}
		return t
	}
	mcpServer.Allowed = func() bool {
		v := getSetting("mcp_server_enabled")
		return v != "false" && v != "0"
	}
	mcpServer.AllowHighRisk = func() bool {
		v := getSetting("mcp_server_allow_high_risk")
		return v == "true" || v == "1"
	}
	mcpServer.Audit = auditSvc.Record

	agentDeps := v1.AgentDeps{
		Agent:    a,
		Store:    store,
		Approver: approver,
		UsernameOf: func(uid uint) string {
			if u, err := userSvc.Get(uid); err == nil {
				return u.Username
			}
			return ""
		},
		PolicyGet: policies,
		PolicySet: savePolicies,
		MCPStatus: func() []aiagent.MCPStatus {
			list, err := d.mcp.List()
			if err != nil {
				return nil
			}
			return mcpMgr.Statuses(list)
		},
		MCPServerToken: mcpServer.Token,
		Settings: v1.Settings{
			Get:    getSetting,
			Set:    settingSvc.Set,
			Ensure: settingSvc.EnsureSecret,
		},
	}

	return aiAgentBundle{
		handler:   v1.NewAgentHandler(agentDeps),
		mcpServer: mcpServer,
	}
}

// agentSnapshot builds a compact live host snapshot for the system prompt, so the
// model can orient itself before deciding which tool to call.
func agentSnapshot(sys *service.SystemService) string {
	if sys == nil {
		return ""
	}
	sum, err := sys.MonitorSummary()
	if err != nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("【当前主机实时快照】\n")
	fmt.Fprintf(&b, "主机名 %s；系统 %s %s；CPU %.1f%%（%d 核）；内存 %.1f%%；磁盘 %.1f%%；负载 %.2f/%.2f/%.2f。\n",
		sum.Hostname, sum.OS, sum.PlatformVer, sum.CPUPercent, sum.CPUCores,
		sum.MemPercent, sum.DiskPercent, sum.Load1, sum.Load5, sum.Load15)
	fmt.Fprintf(&b, "进程 %d（僵尸 %d）；监听端口 %d；主机用户 %d。\n",
		sum.ProcessCount, sum.ZombieCount, sum.PortCount, sum.UserCount)
	b.WriteString("需要更详细的数据时请调用相应工具获取，不要依赖快照做结论。")
	return b.String()
}
