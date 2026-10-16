// Package tools implements the built-in tool set of the OpsMini AI Agent.
//
// Tools are declared exactly like MCP tools (JSON Schema input), which keeps the
// agent portable: the same definitions are exported to the model as OpenAI
// function-calling definitions and to external MCP clients as MCP tools.
package tools

import (
	"github.com/opsmini/opsmini/internal/agent"
	"github.com/opsmini/opsmini/internal/aiagent"
	"github.com/opsmini/opsmini/internal/service"
)

// Deps holds the services the built-in tools operate on. Optional fields may be
// nil (for example Docker when no daemon is available); the corresponding tools
// then degrade gracefully instead of failing the whole registry.
type Deps struct {
	Sys      *service.SystemService
	Firewall *service.FirewallService
	File     *service.FileService
	Log      *service.LogService
	Cron     *service.CronService
	Crontab  *service.CrontabService
	Website  *service.WebsiteService
	Database *service.DatabaseService
	AppStore *service.AppStoreService
	Docker   *service.DockerService // nil when the Docker daemon is unavailable
	Security *service.SecurityService
	Skill    *service.SkillService
	Mcp      *service.McpService
	Metrics  *service.MetricCollector
	Agent    *agent.Manager // remote gRPC peer managed by OpsAnt (M8-E)
	// Remote is the OpsMini Server control-plane client used to drive other
	// enrolled hosts (M8-E). Nil or unconfigured disables the remote tool set.
	Remote *RemoteClient

	// DataDir returns the panel data directory (skills, uploaded files).
	DataDir func() string
	// PanelPort is the port the panel listens on; used by the guard rails.
	PanelPort int
	// Setting reads a runtime panel setting (empty string when unset).
	Setting func(key string) string
}

// Build registers every built-in tool on the registry.
func Build(reg *aiagent.Registry, d Deps) {
	reg.RegisterAll(readonlyTools(d))
	reg.RegisterAll(lowRiskTools(d))
	reg.RegisterAll(highRiskTools(d))
	reg.RegisterAll(remoteTools(d))
}
