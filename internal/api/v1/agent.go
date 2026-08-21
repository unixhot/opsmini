package v1

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/config"
	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// AgentHandler external machine interface (/agent/v1).
type AgentHandler struct {
	cfg       *config.Agent
	sys       *service.SystemService
	websites  *service.WebsiteService
	databases *service.DatabaseService
	crons     *service.CronService
	docker    *service.DockerService // may be nil
	audit     *service.AuditService
}

// NewAgentHandler constructor.
func NewAgentHandler(cfg *config.Agent, sys *service.SystemService, websites *service.WebsiteService,
	databases *service.DatabaseService, crons *service.CronService, docker *service.DockerService,
	audit *service.AuditService) *AgentHandler {
	return &AgentHandler{cfg: cfg, sys: sys, websites: websites, databases: databases, crons: crons, docker: docker, audit: audit}
}

// Health GET /agent/v1/health - health check (external liveness probe).
func (h *AgentHandler) Health(c *gin.Context) {
	response.OK(c, gin.H{"status": "ok", "time": time.Now().Unix()})
}

// Status GET /agent/v1/status - status summary.
func (h *AgentHandler) Status(c *gin.Context) {
	o, err := h.sys.Overview()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, o)
}

// SystemInfo GET /agent/v1/system/info
func (h *AgentHandler) SystemInfo(c *gin.Context) {
	info, err := h.sys.HostInfo()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, info)
}

// Processes GET /agent/v1/system/processes
func (h *AgentHandler) Processes(c *gin.Context) {
	list, err := h.sys.Processes(100)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Ports GET /agent/v1/system/ports
func (h *AgentHandler) Ports(c *gin.Context) {
	list, err := h.sys.Ports()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Disks GET /agent/v1/system/disks
func (h *AgentHandler) Disks(c *gin.Context) {
	list, err := h.sys.Disks()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Websites GET /agent/v1/websites
func (h *AgentHandler) Websites(c *gin.Context) {
	list, err := h.websites.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Databases GET /agent/v1/databases
func (h *AgentHandler) Databases(c *gin.Context) {
	list, err := h.databases.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// CronJobs GET /agent/v1/cron-jobs
func (h *AgentHandler) CronJobs(c *gin.Context) {
	list, err := h.crons.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Containers GET /agent/v1/containers
func (h *AgentHandler) Containers(c *gin.Context) {
	if h.docker == nil {
		response.Error(c, 503, response.CodeInternal, "docker service unavailable")
		return
	}
	list, err := h.docker.Containers()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// commandReq command execution request.
type commandReq struct {
	Command string `json:"command" binding:"required"`
}

// Commands POST /agent/v1/commands - execute a command (whitelist-controlled).
func (h *AgentHandler) Commands(c *gin.Context) {
	var req commandReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if !h.isAllowed(req.Command) {
		response.Error(c, 403, response.CodeForbidden, "command not in whitelist")
		return
	}
	if h.audit != nil {
		h.audit.Record(0, "agent", "command", "system", req.Command, c.ClientIP())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "sh", "-c", req.Command).CombinedOutput()
	if err != nil {
		response.OK(c, gin.H{"stdout": string(out), "stderr": err.Error()})
		return
	}
	response.OK(c, gin.H{"stdout": string(out), "stderr": ""})
}

// isAllowed checks whether the command matches the whitelist (exact command-name match, and blocks shell metacharacter injection).
func (h *AgentHandler) isAllowed(cmd string) bool {
	// Block shell metacharacters to prevent bypassing the whitelist via sh -c injection (e.g. "ps; rm -rf /").
	if strings.ContainsAny(cmd, ";&|`$><()\n\r") {
		return false
	}
	fields := strings.Fields(cmd)
	if len(fields) == 0 {
		return false
	}
	base := filepath.Base(fields[0]) // handle full paths like /usr/bin/xxx
	for _, allowed := range h.cfg.Commands {
		if base == allowed {
			return true
		}
	}
	return false
}
