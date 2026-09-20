package v1

import (
	"io"
	"os"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/service"
)

// FileHandler file management handler.
type FileHandler struct {
	svc *service.FileService
}

// NewFileHandler constructor.
func NewFileHandler(svc *service.FileService) *FileHandler {
	return &FileHandler{svc: svc}
}

// List GET /files?path=/
func (h *FileHandler) List(c *gin.Context) {
	dir := c.DefaultQuery("path", "/")
	list, err := h.svc.List(dir)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// mkdirReq create-directory request payload.
type mkdirReq struct {
	Path string `json:"path"`
	Name string `json:"name" binding:"required"`
}

// MakeDir POST /files/mkdir
func (h *FileHandler) MakeDir(c *gin.Context) {
	var req mkdirReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.MakeDir(req.Path, req.Name); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// renameReq rename request payload.
type renameReq struct {
	Path    string `json:"path" binding:"required"`
	NewName string `json:"new_name" binding:"required"`
}

// Rename POST /files/rename
func (h *FileHandler) Rename(c *gin.Context) {
	var req renameReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.Rename(req.Path, req.NewName); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// deleteReq delete request payload.
type deleteReq struct {
	Path string `json:"path" binding:"required"`
}

// Download GET /files/download?path=...
func (h *FileHandler) Download(c *gin.Context) {
	p := c.Query("path")
	full, name, err := h.svc.Open(p)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	c.FileAttachment(full, name)
}

// Delete POST /files/delete
func (h *FileHandler) Delete(c *gin.Context) {
	var req deleteReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if err := h.svc.Delete(req.Path); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, nil)
}

// Upload POST /files/upload (multipart: file + dir)
func (h *FileHandler) Upload(c *gin.Context) {
	file, err := c.FormFile("file")
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "missing file")
		return
	}
	dir := c.DefaultPostForm("dir", "/")

	src, err := file.Open()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	defer src.Close()
	data, err := io.ReadAll(src)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	if err := h.svc.SaveUploaded(dir, file.Filename, data); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"name": file.Filename, "size": len(data)})
}

// Du GET /files/du?path=... — directory recursive size + filesystem usage (for the download dialog).
func (h *FileHandler) Du(c *gin.Context) {
	p := c.Query("path")
	u, err := h.svc.DirUsage(p)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, u)
}

// DownloadDir GET /files/download-dir?path=... — zip the directory and download it.
func (h *FileHandler) DownloadDir(c *gin.Context) {
	p := c.Query("path")
	tmp, name, err := h.svc.CompressDir(p)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	defer os.Remove(tmp)
	c.FileAttachment(tmp, name)
}
