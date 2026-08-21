package v1

import (
	"io"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// AppStoreHandler app store handler.
type AppStoreHandler struct {
	svc *service.AppStoreService
}

// NewAppStoreHandler constructor.
func NewAppStoreHandler(svc *service.AppStoreService) *AppStoreHandler {
	return &AppStoreHandler{svc: svc}
}

// List GET /apps - app list (with install status).
func (h *AppStoreHandler) List(c *gin.Context) {
	list, err := h.svc.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Get GET /apps/:slug - a single app (with install script).
func (h *AppStoreHandler) Get(c *gin.Context) {
	app, err := h.svc.Get(c.Param("slug"))
	if err != nil {
		response.Error(c, 404, response.CodeNotFound, "app not found")
		return
	}
	response.OK(c, app)
}

// Update PUT /apps/:slug - update an app (config page editing).
func (h *AppStoreHandler) Update(c *gin.Context) {
	app, err := h.svc.Get(c.Param("slug"))
	if err != nil {
		response.Error(c, 404, response.CodeNotFound, "app not found")
		return
	}
	var body struct {
		Name          string `json:"name"`
		Description   string `json:"description"`
		Category      string `json:"category"`
		Version       string `json:"version"`
		Ports         string `json:"ports"`
		ContainerName string `json:"container_name"`
		InstallScript string `json:"install_script"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if body.Name != "" {
		app.Name = body.Name
	}
	if body.Description != "" {
		app.Description = body.Description
	}
	if body.Category != "" {
		app.Category = body.Category
	}
	if body.Version != "" {
		app.Version = body.Version
	}
	if body.Ports != "" {
		app.Ports = body.Ports
	}
	if body.ContainerName != "" {
		app.ContainerName = body.ContainerName
	}
	if body.InstallScript != "" {
		app.InstallScript = body.InstallScript
	}
	if err := h.svc.Update(app); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Install POST /apps/:slug/install - one-click install.
func (h *AppStoreHandler) Install(c *gin.Context) {
	out, err := h.svc.Install(c.Param("slug"))
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"output": out})
}

// Uninstall POST /apps/:slug/uninstall - uninstall.
func (h *AppStoreHandler) Uninstall(c *gin.Context) {
	if err := h.svc.Uninstall(c.Param("slug")); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Start POST /apps/:slug/start - start the app container.
func (h *AppStoreHandler) Start(c *gin.Context) {
	if err := h.svc.Start(c.Param("slug")); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Stop POST /apps/:slug/stop - stop the app container.
func (h *AppStoreHandler) Stop(c *gin.Context) {
	if err := h.svc.Stop(c.Param("slug")); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Restart POST /apps/:slug/restart - restart the app container.
func (h *AppStoreHandler) Restart(c *gin.Context) {
	if err := h.svc.Restart(c.Param("slug")); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Create POST /apps - create an app template.
func (h *AppStoreHandler) Create(c *gin.Context) {
	var app model.App
	if err := c.ShouldBindJSON(&app); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if app.Slug == "" || app.Name == "" {
		response.Error(c, 400, response.CodeInvalidParam, "slug and name are required")
		return
	}
	if err := h.svc.Create(&app); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, app)
}

// Delete DELETE /apps/:slug - delete an app template.
func (h *AppStoreHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Param("slug")); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Export GET /apps/:slug/export - export an app template as a zip.
func (h *AppStoreHandler) Export(c *gin.Context) {
	data, err := h.svc.ExportZip(c.Param("slug"))
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", "attachment; filename=\""+c.Param("slug")+".zip\"")
	c.Data(http.StatusOK, "application/zip", data)
}

// Import POST /apps/import - import an app template zip.
func (h *AppStoreHandler) Import(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "缺少 zip 文件")
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
	app, err := h.svc.ImportZip(data, c.PostForm("slug"), c.PostForm("name"), c.PostForm("category"))
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, app)
}

// Logo GET /apps/:slug/logo - return the template logo.png.
func (h *AppStoreHandler) Logo(c *gin.Context) {
	p := h.svc.LogoPath(c.Param("slug"))
	if p == "" {
		c.Status(http.StatusNotFound)
		return
	}
	c.File(p)
}

// SyncOfficial POST /apps/sync-official - sync official templates from the official app store.
func (h *AppStoreHandler) SyncOfficial(c *gin.Context) {
	count, err := h.svc.SyncOfficial()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"count": count})
}
