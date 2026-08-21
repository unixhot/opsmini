package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// McpHandler MCP configuration handler.
type McpHandler struct {
	svc *service.McpService
}

// NewMcpHandler constructor.
func NewMcpHandler(svc *service.McpService) *McpHandler {
	return &McpHandler{svc: svc}
}

type mcpReq struct {
	Name      string `json:"name" binding:"required"`
	Transport string `json:"transport"`
	Command   string `json:"command"`
	URL       string `json:"url"`
	Enabled   *bool  `json:"enabled"`
}

// List GET /mcp
func (h *McpHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Create POST /mcp
func (h *McpHandler) Create(c *gin.Context) {
	var req mcpReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	m := &model.McpServer{Name: req.Name, Transport: req.Transport, Command: req.Command, URL: req.URL, Enabled: true}
	if req.Transport == "" {
		m.Transport = "stdio"
	}
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	}
	if err := h.svc.Create(m); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, m)
}

// Update PUT /mcp/:id
func (h *McpHandler) Update(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	var req mcpReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	m := &model.McpServer{ID: uint(id), Name: req.Name, Transport: req.Transport, Command: req.Command, URL: req.URL}
	if m.Transport == "" {
		m.Transport = "stdio"
	}
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	} else {
		m.Enabled = true
	}
	if err := h.svc.Update(m); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, m)
}

// Delete DELETE /mcp/:id
func (h *McpHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseUint(c.Param("id"), 10, 64)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}
