package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// FirewallHandler firewall read/write handler.
type FirewallHandler struct {
	svc *service.FirewallService
}

// NewFirewallHandler constructor.
func NewFirewallHandler(svc *service.FirewallService) *FirewallHandler {
	return &FirewallHandler{svc: svc}
}

// Status GET /security/firewall
func (h *FirewallHandler) Status(c *gin.Context) {
	response.OK(c, h.svc.Status())
}

type portReq struct {
	Port  int    `json:"port" binding:"required"`
	Proto string `json:"proto"`
}

// AllowPort POST /security/firewall/port
func (h *FirewallHandler) AllowPort(c *gin.Context) {
	var req portReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if req.Proto == "" {
		req.Proto = "tcp"
	}
	if err := h.svc.AllowPort(req.Port, req.Proto); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// DenyPort DELETE /security/firewall/port
func (h *FirewallHandler) DenyPort(c *gin.Context) {
	var req portReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if req.Proto == "" {
		req.Proto = "tcp"
	}
	if err := h.svc.DenyPort(req.Port, req.Proto); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Enable POST /security/firewall/enable
func (h *FirewallHandler) Enable(c *gin.Context) {
	if err := h.svc.Enable(); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Disable POST /security/firewall/disable
func (h *FirewallHandler) Disable(c *gin.Context) {
	if err := h.svc.Disable(); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// DeleteRule DELETE /security/firewall/rule/:ref
func (h *FirewallHandler) DeleteRule(c *gin.Context) {
	ref := c.Param("ref")
	if ref == "" {
		response.Error(c, 400, response.CodeInvalidParam, "ref is required")
		return
	}
	if err := h.svc.DeleteRule(ref); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}
