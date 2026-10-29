package v1

import (
	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/logpipe"
	"github.com/opsmini/opsmini/internal/pkg/response"
)

// LogPipeHandler exposes the log pipeline as a read-only view.
//
// There is deliberately no create/update endpoint: collection tasks come from
// OpsAnt (one task = one filebeat config file). Letting the panel write tasks too
// would create two sources of truth for the same directory.
type LogPipeHandler struct {
	Mgr *logpipe.Manager
}

// NewLogPipeHandler constructs the handler.
func NewLogPipeHandler(mgr *logpipe.Manager) *LogPipeHandler {
	return &LogPipeHandler{Mgr: mgr}
}

// Status GET /logpipe/status — tasks, counters, warnings, output health.
func (h *LogPipeHandler) Status(c *gin.Context) {
	if h.Mgr == nil {
		response.OK(c, gin.H{"enabled": false, "tasks": []any{}})
		return
	}
	response.OK(c, h.Mgr.Status())
}

// Reload POST /logpipe/reload — force a tasks.d rescan after a manual edit.
func (h *LogPipeHandler) Reload(c *gin.Context) {
	if h.Mgr == nil {
		response.Error(c, 503, response.CodeInternal, "log pipeline is not enabled")
		return
	}
	h.Mgr.ReloadNow()
	response.OK(c, h.Mgr.Status())
}
