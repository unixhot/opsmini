package v1

import (
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// LogHandler log handler.
type LogHandler struct {
	svc *service.LogService
}

// NewLogHandler constructor.
func NewLogHandler(svc *service.LogService) *LogHandler {
	return &LogHandler{svc: svc}
}

// List GET /logs - list readable log sources.
func (h *LogHandler) List(c *gin.Context) {
	response.OK(c, h.svc.List())
}

// Tail GET /logs/tail?path=xxx&lines=100 - read the tail of a log.
func (h *LogHandler) Tail(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		response.Error(c, 400, response.CodeInvalidParam, "path is required")
		return
	}
	lines, _ := strconv.Atoi(c.DefaultQuery("lines", "100"))

	content, err := h.svc.Tail(path, lines)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrLogNotFound):
			response.Error(c, 404, response.CodeNotFound, err.Error())
		case errors.Is(err, service.ErrLogNotAllowed):
			response.Error(c, 403, response.CodeForbidden, err.Error())
		default:
			response.Error(c, 500, response.CodeInternal, err.Error())
		}
		return
	}
	response.OK(c, gin.H{"path": path, "lines": lines, "content": content})
}
