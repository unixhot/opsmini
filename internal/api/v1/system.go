package v1

import (
	"strconv"
	"strings"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// SystemHandler system management handler (host/processes/ports/disks/network).
type SystemHandler struct {
	svc *service.SystemService
}

// NewSystemHandler constructor.
func NewSystemHandler(svc *service.SystemService) *SystemHandler {
	return &SystemHandler{svc: svc}
}

// Info GET /system/info
func (h *SystemHandler) Info(c *gin.Context) {
	info, err := h.svc.HostInfo()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, info)
}

// MonitorSummary GET /system/monitor - aggregated overview for the monitoring page.
func (h *SystemHandler) MonitorSummary(c *gin.Context) {
	sum, err := h.svc.MonitorSummary()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, sum)
}

// Metrics GET /metrics - Prometheus-format metrics (node_exporter compatible).
func (h *SystemHandler) Metrics(c *gin.Context) {
	c.Data(200, "text/plain; version=0.0.4; charset=utf-8", []byte(h.svc.PrometheusMetrics()))
}

// Processes GET /system/processes?limit=100
func (h *SystemHandler) Processes(c *gin.Context) {
	limit := 100
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	list, err := h.svc.Processes(limit)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Ports GET /system/ports
func (h *SystemHandler) Ports(c *gin.Context) {
	list, err := h.svc.Ports()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// killReq kill-process request payload.
type killReq struct {
	PID    int32  `json:"pid" binding:"required"`
	Signal string `json:"signal"` // SIGTERM / SIGKILL / SIGINT / SIGHUP / SIGQUIT (default SIGTERM)
}

// Kill POST /system/process/kill - send a signal to terminate a process.
func (h *SystemHandler) Kill(c *gin.Context) {
	var req killReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	sigMap := map[string]syscall.Signal{
		"SIGTERM": syscall.SIGTERM,
		"SIGKILL": syscall.SIGKILL,
		"SIGINT":  syscall.SIGINT,
		"SIGHUP":  syscall.SIGHUP,
		"SIGQUIT": syscall.SIGQUIT,
	}
	sig := sigMap[strings.ToUpper(strings.TrimSpace(req.Signal))]
	if sig == 0 {
		sig = syscall.SIGTERM
	}
	if err := h.svc.KillProcess(req.PID, sig); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Disks GET /system/disks
func (h *SystemHandler) Disks(c *gin.Context) {
	list, err := h.svc.Disks()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Networks GET /system/network
func (h *SystemHandler) Networks(c *gin.Context) {
	list, err := h.svc.Networks()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Routes GET /system/routes - kernel IP routing table.
func (h *SystemHandler) Routes(c *gin.Context) {
	list, err := h.svc.Routes()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Users GET /system/users - system users (distinguish login/non-login).
func (h *SystemHandler) Users(c *gin.Context) {
	response.OK(c, h.svc.Users())
}

// Groups GET /system/groups - system user groups.
func (h *SystemHandler) Groups(c *gin.Context) {
	response.OK(c, h.svc.Groups())
}

// Firewall GET /system/firewall - system firewall status.
func (h *SystemHandler) Firewall(c *gin.Context) {
	response.OK(c, h.svc.FirewallStatus())
}
