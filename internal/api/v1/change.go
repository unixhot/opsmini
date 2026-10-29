package v1

import (
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/change"
	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/pkg/response"
)

// ChangeHandler serves the v1.4 change-awareness endpoints.
type ChangeHandler struct {
	DB        *gorm.DB
	Store     *change.Store
	Snapshots *change.Snapshots
	Watcher   *change.Watcher
	Correlate *change.Correlator
	// Audit records a human-initiated rollback. Optional.
	Audit func(c *gin.Context, action, resource, detail string)
}

// NewChangeHandler builds the handler.
func NewChangeHandler(db *gorm.DB, store *change.Store, snap *change.Snapshots,
	w *change.Watcher, corr *change.Correlator) *ChangeHandler {
	return &ChangeHandler{DB: db, Store: store, Snapshots: snap, Watcher: w, Correlate: corr}
}

// List GET /changes
func (h *ChangeHandler) List(c *gin.Context) {
	since := time.Time{}
	if v := c.Query("hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			since = time.Now().Add(-time.Duration(n) * time.Hour)
		}
	}
	if v := c.Query("since"); v != "" {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			since = t
		}
	}
	page, _ := strconv.Atoi(c.Query("page"))
	size, _ := strconv.Atoi(c.Query("page_size"))
	list, total, err := h.Store.List(change.Filter{
		Kind:      c.Query("kind"),
		ObjectKey: c.Query("q"),
		Verdict:   c.Query("verdict"),
		Since:     since,
		Page:      page,
		PageSize:  size,
	})
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}

	// Attach verdicts and the analysing incident in two batch queries, so the row can
	// show "可疑 0.72 · 分析中" without the client doing N+1 round trips.
	ids := make([]string, 0, len(list))
	for _, ev := range list {
		ids = append(ids, ev.ID)
	}
	reviews := h.Store.ReviewsFor(ids)
	incidents := h.Store.IncidentForChange(ids)
	incState := map[string]gin.H{}
	if iids := uniqueValues(incidents); len(iids) > 0 {
		var rows []model.Incident
		h.DB.Select("id", "status", "report_rel", "summary").Where("id IN ?", iids).Find(&rows)
		for _, r := range rows {
			incState[r.ID] = gin.H{"status": r.Status, "report_rel": r.ReportRel, "summary": r.Summary}
		}
	}

	response.OK(c, gin.H{
		"list": list, "total": total, "page": page, "page_size": size,
		"reviews": reviews, "change_incidents": incidents, "incident_state": incState,
	})
}

// uniqueValues dedupes a map's values.
func uniqueValues(m map[string]string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(m))
	for _, v := range m {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

// Detail GET /changes/:id
func (h *ChangeHandler) Detail(c *gin.Context) {
	ev, diffs, err := h.Store.Get(c.Param("id"))
	if err != nil {
		response.Error(c, 404, response.CodeNotFound, err.Error())
		return
	}
	body := ""
	for _, d := range diffs {
		body += d.Body
	}
	response.OK(c, gin.H{"change": ev, "diff": body})
}

// Related GET /changes/related?incident_id=xxx
//
// Returns correlation, not causation: the panel and the agent prompt both have to
// phrase it that way.
func (h *ChangeHandler) Related(c *gin.Context) {
	id := c.Query("incident_id")
	if id == "" {
		response.Error(c, 400, response.CodeInvalidParam, "incident_id required")
		return
	}
	var inc model.Incident
	if err := h.DB.Where("id = ?", id).Limit(1).Find(&inc).Error; err != nil || inc.ID == "" {
		response.Error(c, 404, response.CodeNotFound, "incident not found")
		return
	}
	if h.Correlate == nil {
		response.OK(c, gin.H{"list": []any{}, "note": "变更感知未启用"})
		return
	}
	list := h.Correlate.Related(inc, change.DefaultWindow)
	response.OK(c, gin.H{
		"list":  list,
		"note":  "这些只是时间上接近的变更，不等于原因",
		"count": len(list),
	})
}

// Stats GET /changes/stats
func (h *ChangeHandler) Stats(c *gin.Context) {
	usage := h.Store.Usage()
	chain := h.Store.VerifyChain(2000)
	out := gin.H{
		"usage":   usage,
		"chain":   chain,
		"enabled": h.Watcher != nil,
	}
	if h.Watcher != nil {
		out["sources"] = h.Watcher.Status()
		cfg := h.Watcher.Config()
		out["retention"] = gin.H{
			"metadata_days": cfg.Retention.MetadataDays,
			"diff_days":     cfg.Retention.DiffDays,
		}
		out["quota"] = gin.H{"diff_total_size": cfg.Quota.DiffTotalSize}
	}
	response.OK(c, out)
}

// Restore POST /changes/:id/restore
//
// Rolling back rewrites a live configuration file, so it needs the write
// permission AND an explicit confirm token. It also keeps a copy of the current
// state first: if the rollback makes things worse there is still a way back.
func (h *ChangeHandler) Restore(c *gin.Context) {
	var req struct {
		Confirm string `json:"confirm"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Confirm != "RESTORE" {
		response.Error(c, 400, response.CodeInvalidParam, "需要 confirm=\"RESTORE\" 确认")
		return
	}
	id := c.Param("id")
	ev, _, err := h.Store.Get(id)
	if err != nil {
		response.Error(c, 404, response.CodeNotFound, err.Error())
		return
	}
	if ev.Kind != model.ChangeKindConfig && ev.Kind != model.ChangeKindAccount {
		response.Error(c, 400, response.CodeInvalidParam, "该类型变更不支持回滚")
		return
	}
	if h.Snapshots == nil {
		response.Error(c, 400, response.CodeInvalidParam, "快照未启用")
		return
	}
	path, err := h.Snapshots.Restore(id)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "回滚失败："+err.Error())
		return
	}
	if h.Audit != nil {
		h.Audit(c, "change.restore", path, "回滚变更 "+id+" 到变更前状态")
	}
	response.OK(c, gin.H{"path": path, "restored_from": id})
}

// Verify POST /changes/verify
func (h *ChangeHandler) Verify(c *gin.Context) {
	response.OK(c, h.Store.VerifyChain(5000))
}

// ListIgnore GET /changes/ignore
func (h *ChangeHandler) ListIgnore(c *gin.Context) {
	response.OK(c, gin.H{"list": h.Store.ListIgnore(), "blocked": blockedHighlights()})
}

// AddIgnore POST /changes/ignore
func (h *ChangeHandler) AddIgnore(c *gin.Context) {
	var req struct {
		Pattern string `json:"pattern"`
		Note    string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, "参数错误")
		return
	}
	if strings.TrimSpace(req.Pattern) == "" {
		response.Error(c, 400, response.CodeInvalidParam, "pattern required")
		return
	}
	if err := h.Store.AddIgnore(req.Pattern, req.Note); err != nil {
		// A duplicate is a user mistake, not a server fault.
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, gin.H{"ok": true})
}

// DeleteIgnore DELETE /changes/ignore/:id
func (h *ChangeHandler) DeleteIgnore(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.Store.RemoveIgnore(uint(id)); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"ok": true})
}

// blockedHighlights tells the UI why certain paths can never be collected. These
// are compiled in and cannot be removed through configuration.
func blockedHighlights() []string {
	return []string{
		"/etc/shadow, /etc/gshadow（密码哈希不采集，只记变化）",
		"*.key, *.pem, *.p12（私钥与证书不读）",
		"/root/.ssh/**, /root/.bash_history（凭据与命令历史）",
		"/data/opsmini/**（自身数据，防自激）",
	}
}
