package v1

import (
	"context"
	"errors"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/agent"
	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// AgentConfigHandler handles the OpsAnt connection configuration
// (panel visualization of the Agent's OpsAnt "OpsMini Server" link).
type AgentConfigHandler struct {
	svc *service.AgentConfigService
}

// NewAgentConfigHandler constructor.
func NewAgentConfigHandler(svc *service.AgentConfigService) *AgentConfigHandler {
	return &AgentConfigHandler{svc: svc}
}

// Get GET /agent-config - returns the current server_addr / server_token and running state.
func (h *AgentConfigHandler) Get(c *gin.Context) {
	addr, token, running, connected, linkErr := h.svc.Current()
	response.OK(c, gin.H{
		"server_addr":  addr,
		"server_token": token,
		"running":      running,
		"connected":    connected,
		// link_error / link_hint expose why a saved config still isn't connected.
		// The raw gRPC text alone is unreadable; the hint names the fix.
		"link_error": linkErr,
		"link_hint":  agentExplain(linkErr),
	})
}

// agentExplain maps a raw link error string back to its operator-facing hint.
func agentExplain(raw string) string {
	if raw == "" {
		return ""
	}
	return agent.ExplainConnectError(errors.New(raw))
}

// Update PUT /agent-config - persists the configuration and applies it immediately.
func (h *AgentConfigHandler) Update(c *gin.Context) {
	var req struct {
		ServerAddr  string `json:"server_addr"`
		ServerToken string `json:"server_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.Save(req.ServerAddr, req.ServerToken); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Test POST /agent-config/test - verifies the typed address + token without
// saving them, so an operator can tell whether the values work before
// committing. Nothing is persisted and the live agent keeps running.
func (h *AgentConfigHandler) Test(c *gin.Context) {
	var req struct {
		ServerAddr  string `json:"server_addr"`
		ServerToken string `json:"server_token"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	// Probe budget is 4s dial + 8s handshake; leave headroom for the HTTP layer
	// so a slow network returns our structured reason instead of a client abort.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	response.OK(c, h.svc.Probe(ctx, req.ServerAddr, req.ServerToken))
}
