package v1

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// FimHandler file integrity monitoring handler.
type FimHandler struct {
	svc *service.FimService
}

// NewFimHandler constructor.
func NewFimHandler(svc *service.FimService) *FimHandler {
	return &FimHandler{svc: svc}
}

// ListBaselines GET /security/fim/baselines
func (h *FimHandler) ListBaselines(c *gin.Context) {
	list, err := h.svc.ListBaselines()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Rebuild POST /security/fim/rebuild
func (h *FimHandler) Rebuild(c *gin.Context) {
	if err := h.svc.Rebuild(); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// RemoveBaseline DELETE /security/fim/baselines/:id
func (h *FimHandler) RemoveBaseline(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	if err := h.svc.RemoveBaseline(uint(id)); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// ListChanges GET /security/fim/events
func (h *FimHandler) ListChanges(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	list, err := h.svc.ListChanges(limit)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Paths GET /security/fim/paths
//
// Returns the configured patterns plus what they currently expand to. The
// expansion matters: "/etc/**/*.conf" silently matching nothing looks exactly
// like "no changes happened", and only the resolved count tells them apart.
func (h *FimHandler) Paths(c *gin.Context) {
	patterns, critical := h.svc.MonitoredPaths()
	response.OK(c, gin.H{
		"paths":    patterns,
		"critical": critical,
		"resolved": h.svc.ResolveForDisplay(patterns),
	})
}

// UpdatePaths PUT /security/fim/paths
func (h *FimHandler) UpdatePaths(c *gin.Context) {
	var req struct {
		Paths    []string `json:"paths"`
		Critical []string `json:"critical"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	clean := func(in []string) string {
		out := make([]string, 0, len(in))
		for _, v := range in {
			if p := strings.TrimSpace(v); p != "" {
				out = append(out, p)
			}
		}
		return strings.Join(out, ",")
	}
	paths := clean(req.Paths)
	if paths == "" {
		response.Error(c, 400, response.CodeInvalidParam, "至少需要一个监控路径")
		return
	}
	if err := h.svc.SavePaths(paths, clean(req.Critical)); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	// Rebuild immediately: the old baseline refers to files that may no longer be
	// monitored, and leaving it would report phantom deletions on the next round.
	if err := h.svc.Rebuild(); err != nil {
		response.Error(c, 500, response.CodeInternal, "路径已保存，但重建基线失败："+err.Error())
		return
	}
	patterns, critical := h.svc.MonitoredPaths()
	response.OK(c, gin.H{"paths": patterns, "critical": critical, "resolved": h.svc.ResolveForDisplay(patterns)})
}
