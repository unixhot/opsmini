package v1

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/plugin"
)

// PluginHandler exposes the plugin runtime.
//
// Installing from the panel is intentionally a package upload (with digest and
// signature) rather than "give me a URL": the panel must never fetch code on its
// own, and every install has to be verifiable.
type PluginHandler struct {
	Mgr *plugin.Manager
}

// NewPluginHandler constructs the handler.
func NewPluginHandler(mgr *plugin.Manager) *PluginHandler {
	return &PluginHandler{Mgr: mgr}
}

// List GET /plugins
func (h *PluginHandler) List(c *gin.Context) {
	if h.Mgr == nil {
		response.OK(c, gin.H{"plugins": []any{}, "events": []any{}})
		return
	}
	response.OK(c, gin.H{
		"plugins": h.Mgr.List(),
		"events":  h.Mgr.Events(50),
	})
}

// Install POST /plugins/install  multipart: package=<file>, sha256=<hex>, signature=<b64>
func (h *PluginHandler) Install(c *gin.Context) {
	if h.Mgr == nil {
		response.Error(c, 503, response.CodeInternal, "plugin runtime is not enabled")
		return
	}
	fh, err := c.FormFile("package")
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "缺少 package 文件")
		return
	}
	if fh.Size > 64<<20 {
		response.Error(c, 400, response.CodeInvalidParam, "插件包超过 64MB")
		return
	}
	src, err := fh.Open()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	defer src.Close()

	dir, err := os.MkdirTemp("", "opsmini-pkg")
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	defer os.RemoveAll(dir)
	// Keep the original file name so the log line names the package.
	name := filepath.Base(fh.Filename)
	if name == "" || name == "." {
		name = "plugin.tar.gz"
	}
	path := filepath.Join(dir, name)
	dst, err := os.Create(path)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	hsh := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dst, hsh), io.LimitReader(src, 64<<20)); err != nil {
		dst.Close()
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	dst.Close()

	// The digest is computed server-side; a client-supplied one is only used to
	// cross-check what the uploader claims (guards against truncated uploads).
	actual := hex.EncodeToString(hsh.Sum(nil))
	if want := c.PostForm("sha256"); want != "" && !equalFold(want, actual) {
		response.Error(c, 400, response.CodeInvalidParam, "上传内容与声明的 sha256 不一致")
		return
	}
	man, err := h.Mgr.Install(path, actual, c.PostForm("signature"))
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, gin.H{"id": man.ID, "version": man.Version, "name": man.Name})
}

// Enable POST /plugins/:id/enable  {"enabled": true}
func (h *PluginHandler) Enable(c *gin.Context) {
	if h.Mgr == nil {
		response.Error(c, 503, response.CodeInternal, "plugin runtime is not enabled")
		return
	}
	var req struct {
		Enabled bool `json:"enabled"`
	}
	_ = c.ShouldBindJSON(&req)
	if err := h.Mgr.Enable(c.Param("id"), req.Enabled); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, nil)
}

// Restart POST /plugins/:id/restart
func (h *PluginHandler) Restart(c *gin.Context) {
	if h.Mgr == nil {
		response.Error(c, 503, response.CodeInternal, "plugin runtime is not enabled")
		return
	}
	if err := h.Mgr.Restart(c.Param("id")); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, nil)
}

// Delete DELETE /plugins/:id?purge=1
func (h *PluginHandler) Delete(c *gin.Context) {
	if h.Mgr == nil {
		response.Error(c, 503, response.CodeInternal, "plugin runtime is not enabled")
		return
	}
	purge, _ := strconv.ParseBool(c.DefaultQuery("purge", "false"))
	if err := h.Mgr.Remove(c.Param("id"), purge); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, nil)
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if 'A' <= ca && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if 'A' <= cb && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
