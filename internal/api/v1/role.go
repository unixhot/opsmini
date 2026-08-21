package v1

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// RoleHandler role and permission handler.
type RoleHandler struct {
	svc *service.RoleService
}

// NewRoleHandler constructor.
func NewRoleHandler(svc *service.RoleService) *RoleHandler {
	return &RoleHandler{svc: svc}
}

// List GET /roles - role list (admin).
func (h *RoleHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Groups GET /roles/groups - permission groups (all logged-in users, for frontend display).
func (h *RoleHandler) Groups(c *gin.Context) {
	response.OK(c, h.svc.Groups())
}

// MyPerms GET /permissions - permission points of the current logged-in user.
func (h *RoleHandler) MyPerms(c *gin.Context) {
	role, _ := c.Get("role")
	r, _ := role.(string)
	response.OK(c, h.svc.PermsOf(r))
}

type roleReq struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Perms string `json:"perms"`
}

// Create POST /roles - create a custom role (admin).
func (h *RoleHandler) Create(c *gin.Context) {
	var req roleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	role, err := h.svc.Create(req.Name, req.Label, req.Perms)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, role)
}

// Update PUT /roles/:id - update a role (admin).
func (h *RoleHandler) Update(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	var req roleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.Update(uint(id), req.Name, req.Label, req.Perms); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, nil)
}

// Delete DELETE /roles/:id - delete a custom role (admin).
func (h *RoleHandler) Delete(c *gin.Context) {
	id, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "invalid id")
		return
	}
	if err := h.svc.Delete(uint(id)); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, nil)
}
