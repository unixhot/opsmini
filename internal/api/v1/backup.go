package v1

import (
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/backup"
	"github.com/opsmini/opsmini/internal/pkg/response"
)

// BackupHandler runs on-demand backups and restores.
//
// Objects are entered as a simple list rather than a structured editor: one line
// per object, prefixed by kind.
//
//	/var/www/html            directory
//	mysql:opsmini            database
//	redis:/var/lib/redis/dump.rdb
//
// A structured editor can come later; the engine already takes typed objects, so
// this only decides how the panel expresses them.
type BackupHandler struct {
	Mgr *backup.Manager
}

// NewBackupHandler constructs the handler.
func NewBackupHandler(mgr *backup.Manager) *BackupHandler {
	return &BackupHandler{Mgr: mgr}
}

type backupRunReq struct {
	Name      string `json:"name"`
	Objects   string `json:"objects"`
	Retention int    `json:"retention"`
	Database  string `json:"database_user"`
	Password  string `json:"database_password"`
}

// Snapshots GET /backup/snapshots
func (h *BackupHandler) Snapshots(c *gin.Context) {
	if h.Mgr == nil {
		response.OK(c, []any{})
		return
	}
	list, err := h.Mgr.List("")
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"snapshots": list, "root": h.Mgr.Root()})
}

// Run POST /backup/run
func (h *BackupHandler) Run(c *gin.Context) {
	if h.Mgr == nil {
		response.Error(c, 503, response.CodeInternal, "备份模块未启用")
		return
	}
	var req backupRunReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	objs := parseBackupObjects(req.Objects, req.Database, req.Password)
	if len(objs) == 0 {
		response.Error(c, 400, response.CodeInvalidParam, "至少需要一个备份对象（目录或 mysql:库名）")
		return
	}
	man, err := h.Mgr.Run(backup.Job{
		Name:      orDefaultStr(req.Name, "manual"),
		Objects:   objs,
		Retention: req.Retention,
	})
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, man)
}

type restoreReq struct {
	Snapshot string `json:"snapshot"`
	Confirm  string `json:"confirm"`
	DryRun   bool   `json:"dry_run"`
}

// Restore POST /backup/restore
func (h *BackupHandler) Restore(c *gin.Context) {
	if h.Mgr == nil {
		response.Error(c, 503, response.CodeInternal, "备份模块未启用")
		return
	}
	var req restoreReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	dir, err := h.Mgr.SnapshotDir(req.Snapshot)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	res, err := h.Mgr.Restore(backup.RestoreOpts{SnapshotDir: dir, Confirm: req.Confirm, DryRun: req.DryRun})
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, res)
}

// parseBackupObjects turns the panel's line syntax into typed objects.
func parseBackupObjects(raw, dbUser, dbPass string) []backup.Object {
	var out []backup.Object
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		switch {
		case strings.HasPrefix(line, "mysql:"):
			name := strings.TrimSpace(strings.TrimPrefix(line, "mysql:"))
			if name == "" {
				continue
			}
			out = append(out, backup.Object{Kind: "mysql", Database: name, Name: "mysql-" + name, User: dbUser, Password: dbPass})
		case strings.HasPrefix(line, "redis:"):
			path := strings.TrimSpace(strings.TrimPrefix(line, "redis:"))
			out = append(out, backup.Object{Kind: "redis", Path: path, Name: "redis"})
		default:
			out = append(out, backup.Object{Kind: "dir", Path: line, Name: strings.Trim(strings.ReplaceAll(line, "/", "_"), "_")})
		}
	}
	return out
}

func orDefaultStr(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

var _ = strconv.Itoa
