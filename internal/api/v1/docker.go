package v1

import (
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"

	"github.com/opsmini/opsmini/internal/pkg/jwt"
	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// DockerHandler container management handler.
type DockerHandler struct {
	svc      *service.DockerService
	jwtMgr   *jwt.Manager
	hasPerm  func(role, perm string) bool
	upgrader websocket.Upgrader
}

// NewDockerHandler constructor. jwtMgr/hasPerm are used for WebSocket authentication of the container exec terminal.
func NewDockerHandler(svc *service.DockerService, jwtMgr *jwt.Manager, hasPerm func(role, perm string) bool) *DockerHandler {
	return &DockerHandler{
		svc:     svc,
		jwtMgr:  jwtMgr,
		hasPerm: hasPerm,
		upgrader: websocket.Upgrader{
			// Only allow same-origin connections to prevent cross-site WebSocket hijacking (CSWSH).
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				if origin == "" {
					return true
				}
				u, err := url.Parse(origin)
				if err != nil {
					return false
				}
				return strings.EqualFold(u.Host, r.Host)
			},
		},
	}
}

// Containers GET /containers
func (h *DockerHandler) Containers(c *gin.Context) {
	list, err := h.svc.Containers()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// StartContainer POST /containers/:id/start
func (h *DockerHandler) StartContainer(c *gin.Context) {
	if err := h.svc.StartContainer(c.Param("id")); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// StopContainer POST /containers/:id/stop
func (h *DockerHandler) StopContainer(c *gin.Context) {
	if err := h.svc.StopContainer(c.Param("id")); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// RestartContainer POST /containers/:id/restart
func (h *DockerHandler) RestartContainer(c *gin.Context) {
	if err := h.svc.RestartContainer(c.Param("id")); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// RemoveContainer DELETE /containers/:id
func (h *DockerHandler) RemoveContainer(c *gin.Context) {
	if err := h.svc.RemoveContainer(c.Param("id")); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Logs GET /containers/:id/logs?tail=200 - get container logs.
func (h *DockerHandler) Logs(c *gin.Context) {
	tail := 200
	if t, err := strconv.Atoi(c.Query("tail")); err == nil && t > 0 {
		tail = t
	}
	logs, err := h.svc.ContainerLogs(c.Param("id"), tail)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"logs": logs})
}

// Inspect GET /containers/:id - get container details (basic info/network/volumes).
func (h *DockerHandler) Inspect(c *gin.Context) {
	detail, err := h.svc.Inspect(c.Param("id"))
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, detail)
}

// Images GET /images
func (h *DockerHandler) Images(c *gin.Context) {
	list, err := h.svc.Images()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// RemoveImage DELETE /images/:id
func (h *DockerHandler) RemoveImage(c *gin.Context) {
	if err := h.svc.RemoveImage(c.Param("id")); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Volumes GET /volumes
func (h *DockerHandler) Volumes(c *gin.Context) {
	list, err := h.svc.Volumes()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// RemoveVolume DELETE /volumes/:id
func (h *DockerHandler) RemoveVolume(c *gin.Context) {
	if err := h.svc.RemoveVolume(c.Param("id")); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Networks GET /networks
func (h *DockerHandler) Networks(c *gin.Context) {
	list, err := h.svc.Networks()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// RemoveNetwork DELETE /networks/:id
func (h *DockerHandler) RemoveNetwork(c *gin.Context) {
	if err := h.svc.RemoveNetwork(c.Param("id")); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// PullImage POST /images/pull
func (h *DockerHandler) PullImage(c *gin.Context) {
	var req struct {
		Ref string `json:"ref" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.PullImage(req.Ref); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// CreateVolume POST /volumes
func (h *DockerHandler) CreateVolume(c *gin.Context) {
	var req struct {
		Name   string `json:"name" binding:"required"`
		Driver string `json:"driver"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.CreateVolume(req.Name, req.Driver); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// CreateNetwork POST /networks
func (h *DockerHandler) CreateNetwork(c *gin.Context) {
	var req struct {
		Name    string `json:"name" binding:"required"`
		Driver  string `json:"driver"`
		Subnet  string `json:"subnet"`
		Gateway string `json:"gateway"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.CreateNetwork(req.Name, req.Driver, req.Subnet, req.Gateway); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}
