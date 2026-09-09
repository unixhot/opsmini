package v1

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	version   string
	sys       *service.SystemService
	websites  *service.WebsiteService
	databases *service.DatabaseService
	crons     *service.CronService
	docker    *service.DockerService // may be nil
	audit     *service.AuditService
}

// NewAgentHandler constructor.
func NewAgentHandler(cfg *config.Agent, version string, sys *service.SystemService, websites *service.WebsiteService,
	databases *service.DatabaseService, crons *service.CronService, docker *service.DockerService,
	audit *service.AuditService) *AgentHandler {
	return &AgentHandler{cfg: cfg, version: version, sys: sys, websites: websites, databases: databases,
		crons: crons, docker: docker, audit: audit}
}

// Health GET /agent/v1/health - health check (external liveness probe).
func (h *AgentHandler) Health(c *gin.Context) {
	response.OK(c, gin.H{"status": "ok", "time": time.Now().Unix(), "version": h.version})
}

// Version GET /agent/v1/version - build version info.
func (h *AgentHandler) Version(c *gin.Context) {
	response.OK(c, gin.H{"version": h.version})
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
	// Also block tab/vertical-tab/form-feed: they act as command separators in POSIX shell,
	// so "ps<TAB>whoami" would otherwise slip past the first-field check.
	if strings.ContainsAny(cmd, ";&|`$><()\t\n\r\v\f") {
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

// scriptReq script execution request.
type scriptReq struct {
	Script   string `json:"script" binding:"required"`
	Language string `json:"language"` // shell | python
	Timeout  int    `json:"timeout"`  // seconds, 1-3600
}

// ScriptRun POST /agent/v1/script/run - execute a shell/python script (full script,
// NOT whitelist-controlled; caller OpsAnt authenticates via agent token and audits).
func (h *AgentHandler) ScriptRun(c *gin.Context) {
	var req scriptReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	language := req.Language
	if language == "" {
		language = "shell"
	}
	if language != "shell" && language != "python" {
		response.Error(c, 400, response.CodeInvalidParam, "language must be shell or python")
		return
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 300
	}
	if timeout > 3600 {
		timeout = 3600
	}

	// Write the script to a temp file to avoid shell quoting/arg-length issues.
	ext := ".sh"
	if language == "python" {
		ext = ".py"
	}
	tmp, err := os.CreateTemp("", "agent-script-*"+ext)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, "create temp file: "+err.Error())
		return
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(req.Script); err != nil {
		tmp.Close()
		response.Error(c, 500, response.CodeInternal, "write temp file: "+err.Error())
		return
	}
	tmp.Close()

	if h.audit != nil {
		detail := req.Script
		if len(detail) > 500 {
			detail = detail[:500]
		}
		h.audit.Record(0, "agent", "script_run", "system", language+": "+detail, c.ClientIP())
	}

	var cmd *exec.Cmd
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	if language == "python" {
		cmd = exec.CommandContext(ctx, "python3", tmpPath)
	} else {
		cmd = exec.CommandContext(ctx, "sh", tmpPath)
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	runErr := cmd.Run()
	duration := time.Since(start).Milliseconds()

	exitCode := 0
	if runErr != nil {
		if ctx.Err() == context.DeadlineExceeded {
			exitCode = -1
			stderr.WriteString(fmt.Sprintf("\nexecution timed out after %ds", timeout))
		} else if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
			stderr.WriteString(runErr.Error())
		}
	}
	response.OK(c, gin.H{
		"stdout":    stdout.String(),
		"stderr":    stderr.String(),
		"exit_code": exitCode,
		"duration":  duration,
	})
}

// FileUpload POST /agent/v1/file/upload - receive a distributed file and save it to dest_path.
// Form fields: file (required), dest_path (required, absolute path or directory), mode (optional, octal like 0755).
func (h *AgentHandler) FileUpload(c *gin.Context) {
	destPath := strings.TrimSpace(c.PostForm("dest_path"))
	modeStr := strings.TrimSpace(c.PostForm("mode"))
	fileHeader, err := c.FormFile("file")
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "missing file field")
		return
	}
	if destPath == "" {
		response.Error(c, 400, response.CodeInvalidParam, "dest_path is required")
		return
	}
	if !filepath.IsAbs(destPath) {
		response.Error(c, 400, response.CodeInvalidParam, "dest_path must be an absolute path")
		return
	}

	// If dest_path is an existing directory, save under it with the original filename.
	finalPath := destPath
	if st, err := os.Stat(destPath); err == nil && st.IsDir() {
		// Strip any path components from the client-supplied filename to prevent directory traversal.
		finalPath = filepath.Join(destPath, filepath.Base(fileHeader.Filename))
	}
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o755); err != nil {
		response.Error(c, 500, response.CodeInternal, "mkdir: "+err.Error())
		return
	}

	src, err := fileHeader.Open()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, "open upload: "+err.Error())
		return
	}
	defer src.Close()
	dst, err := os.OpenFile(finalPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, "create file: "+err.Error())
		return
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		response.Error(c, 500, response.CodeInternal, "write file: "+err.Error())
		return
	}
	dst.Close()

	mode := ""
	if modeStr != "" {
		m, err := strconv.ParseUint(modeStr, 8, 32)
		if err != nil {
			response.Error(c, 400, response.CodeInvalidParam, "invalid mode (expect octal like 0755)")
			return
		}
		if err := os.Chmod(finalPath, os.FileMode(m)); err != nil {
			response.Error(c, 500, response.CodeInternal, "chmod: "+err.Error())
			return
		}
		mode = fmt.Sprintf("%04o", os.FileMode(m).Perm())
	}

	if h.audit != nil {
		h.audit.Record(0, "agent", "file_upload", "system",
			fmt.Sprintf("%s -> %s (%d bytes)", fileHeader.Filename, finalPath, fileHeader.Size), c.ClientIP())
	}
	response.OK(c, gin.H{"path": finalPath, "size": fileHeader.Size, "mode": mode})
}
