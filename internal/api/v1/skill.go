package v1

import (
	"io"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// SkillHandler skill handler.
type SkillHandler struct {
	svc *service.SkillService
}

// NewSkillHandler constructor.
func NewSkillHandler(svc *service.SkillService) *SkillHandler {
	return &SkillHandler{svc: svc}
}

// List GET /skills - installed skills.
func (h *SkillHandler) List(c *gin.Context) {
	response.OK(c, h.svc.List())
}

// Catalog GET /skills/catalog - recommended skills (SkillHub by rating).
func (h *SkillHandler) Catalog(c *gin.Context) {
	list, err := h.svc.Catalog()
	if err != nil {
		response.Error(c, 502, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Search GET /skills/search?keyword=&category=&sortBy=&page=&pageSize= - search SkillHub.
func (h *SkillHandler) Search(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	list, err := h.svc.Search(c.Query("keyword"), c.Query("category"), c.Query("sortBy"), page, pageSize)
	if err != nil {
		response.Error(c, 502, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// installReq one-click install request.
type installReq struct {
	Slug string `json:"slug" binding:"required"`
}

// Install POST /skills/install - download and install from SkillHub.
func (h *SkillHandler) Install(c *gin.Context) {
	var req installReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.Install(req.Slug); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, nil)
}

// Create POST /skills - create a skill (generate a SKILL.md template).
func (h *SkillHandler) Create(c *gin.Context) {
	var req struct {
		Name        string `json:"name" binding:"required"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.Create(req.Name, req.Description); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, nil)
}

// Upload POST /skills/upload - upload a zip to install a skill.
func (h *SkillHandler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "缺少文件")
		return
	}
	f, err := file.Open()
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.InstallFromZip(data); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, nil)
}

// Delete DELETE /skills/:name - delete a skill.
func (h *SkillHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Param("name")); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, nil)
}

// Toggle POST /skills/:name/toggle - toggle enable/disable.
func (h *SkillHandler) Toggle(c *gin.Context) {
	enabled, err := h.svc.Toggle(c.Param("name"))
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, gin.H{"enabled": enabled})
}
