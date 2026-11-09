package v1

import (
	"encoding/json"
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
	// Reload rebuilds the watcher after the collection settings changed. The
	// handler cannot do it itself: sources start their goroutines in Start() and
	// cannot be switched off individually, so the whole watcher is replaced.
	Reload func() error
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
	// Fallback for changes whose signal row is gone: signals are pruned after the
	// retention window, and with them the change→incident pointer, which would make
	// the verdict column silently lose its "analysed / in attack chain" state a week
	// later. The grouping key already encodes the relationship
	// (group_key = "change|"+object), so the latest incident for the same object is
	// the right answer whenever the exact pointer is missing.
	if len(incidents) < len(ids) {
		byObject := make([]string, 0, len(list))
		pending := map[string][]string{}
		for _, ev := range list {
			if _, ok := incidents[ev.ID]; ok || ev.ObjectKey == "" {
				continue
			}
			byObject = append(byObject, ev.ObjectKey)
			pending[ev.ObjectKey] = append(pending[ev.ObjectKey], ev.ID)
		}
		if len(byObject) > 0 {
			var rows []model.Incident
			h.DB.Select("id", "subject").Where("family = ? AND subject IN ?", model.FamilyChange, byObject).
				Order("last_seen DESC").Find(&rows)
			for _, r := range rows {
				for _, cid := range pending[r.Subject] {
					if _, ok := incidents[cid]; !ok {
						incidents[cid] = r.ID
					}
				}
			}
		}
	}
	incState := map[string]gin.H{}
	campaigns := map[string]gin.H{}
	if iids := uniqueValues(incidents); len(iids) > 0 {
		var rows []model.Incident
		h.DB.Select("id", "status", "report_rel", "summary", "campaign_id").
			Where("id IN ?", iids).Find(&rows)
		cids := make([]string, 0, len(rows))
		for _, r := range rows {
			incState[r.ID] = gin.H{"status": r.Status, "report_rel": r.ReportRel,
				"summary": r.Summary, "campaign_id": r.CampaignID}
			if r.CampaignID != "" {
				cids = append(cids, r.CampaignID)
			}
		}
		// A change that became a stage of an attack chain must say so in the change
		// list, otherwise the operator reads "可疑 0.90" and never learns it was
		// already correlated with the webshell twenty minutes earlier.
		if len(cids) > 0 {
			var camps []model.Campaign
			h.DB.Select("id", "title", "severity", "stage_count").
				Where("id IN ?", cids).Find(&camps)
			for _, cp := range camps {
				campaigns[cp.ID] = gin.H{"id": cp.ID, "title": cp.Title,
					"severity": cp.Severity, "stage_count": cp.StageCount}
			}
		}
	}

	response.OK(c, gin.H{
		"list": list, "total": total, "page": page, "page_size": size,
		"reviews": reviews, "change_incidents": incidents, "incident_state": incState,
		"campaigns": campaigns,
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

// Timeline GET /changes/timeline?hours=24&bucket=1h
//
// Aggregates the change stream into time slots for the timeline view. It exists
// because the list endpoint answers "what changed" but not "when did it get
// busy", and a shape is what makes an unusual night visible.
func (h *ChangeHandler) Timeline(c *gin.Context) {
	if h.Store == nil {
		response.OK(c, change.Timeline{})
		return
	}
	hours := 24
	if v := c.Query("hours"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 24*30 {
			hours = n
		}
	}
	bucket := time.Hour
	switch strings.ToLower(c.DefaultQuery("bucket", "1h")) {
	case "5m":
		bucket = 5 * time.Minute
	case "15m":
		bucket = 15 * time.Minute
	case "1h":
		bucket = time.Hour
	case "1d":
		bucket = 24 * time.Hour
	default:
		response.Error(c, 400, response.CodeInvalidParam, "bucket 只能是 5m / 15m / 1h / 1d")
		return
	}
	// A window finer than the bucket would produce a single bar; that is a
	// caller mistake worth naming rather than silently rounding.
	if bucket > time.Duration(hours)*time.Hour {
		response.Error(c, 400, response.CodeInvalidParam, "bucket 大于查询窗口，请改用更小的 bucket 或更长的时间范围")
		return
	}
	to := time.Now()
	from := to.Add(-time.Duration(hours) * time.Hour)
	response.OK(c, h.Store.Timeline(from, to, bucket, 5000))
}

// changeSourceKeys is the canonical order of the collection switches. Keeping it
// in one place means the frontend and the backend cannot drift apart on names.
var changeSourceKeys = []string{"file", "package", "service", "account", "firewall", "cron"}

// changeSettingsReq is the body of PUT /changes/settings.
type changeSettingsReq struct {
	Sources      map[string]bool `json:"sources"`
	WatchDirs    []string        `json:"watch_dirs"`
	ExcludeGlobs []string        `json:"exclude_globs"`
	// Realtime is a pointer so "not sent" differs from "explicitly false".
	Realtime *bool `json:"realtime"`
}

// ChangeSettings GET /changes/settings
//
// Returns both the stored wishes and the live state: after a save the operator
// needs to see whether the new configuration actually took effect, and "saved"
// is not the same as "running".
func (h *ChangeHandler) ChangeSettings(c *gin.Context) {
	sources := map[string]bool{}
	if raw := h.settingValue("change_sources"); raw != "" {
		_ = json.Unmarshal([]byte(raw), &sources)
	}
	for _, k := range changeSourceKeys {
		if _, ok := sources[k]; !ok {
			sources[k] = true // absent = default on, never "collect nothing"
		}
	}
	realtime := h.settingValue("change_realtime") != "false" && h.settingValue("change_realtime") != "0"

	status := []change.SourceStatus{}
	// Show the directories actually in effect, not the stored override: an empty
	// setting means "the defaults", and an empty text box would look like
	// "watching nothing".
	watchDirs := csvField(h.settingValue("change_watch_dirs"))
	if len(watchDirs) == 0 && h.Watcher != nil {
		if c := h.Watcher.Config(); c != nil {
			watchDirs = c.File.WatchDirs
		}
	}
	if h.Watcher != nil {
		status = h.Watcher.Status()
	}
	response.OK(c, gin.H{
		"sources":       sources,
		"watch_dirs":    watchDirs,
		"exclude_globs": csvField(h.settingValue("change_exclude_globs")),
		"realtime":      realtime,
		"status":        status,
		"running":       h.Watcher != nil,
	})
}

// UpdateChangeSettings PUT /changes/settings
func (h *ChangeHandler) UpdateChangeSettings(c *gin.Context) {
	var req changeSettingsReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	for k := range req.Sources {
		known := false
		for _, want := range changeSourceKeys {
			if k == want {
				known = true
				break
			}
		}
		if !known {
			response.Error(c, 400, response.CodeInvalidParam, "未知的采集源："+k)
			return
		}
	}
	// Refuse an all-off configuration only when it was sent explicitly: it is a
	// legal choice, but almost always a mis-click, and the symptom (silence) looks
	// exactly like "nothing is happening on this host".
	if len(req.Sources) == len(changeSourceKeys) {
		anyOn := false
		for _, v := range req.Sources {
			if v {
				anyOn = true
				break
			}
		}
		if !anyOn {
			response.Error(c, 400, response.CodeInvalidParam, "至少需要开启一个采集源")
			return
		}
	}

	kv := map[string]string{}
	if len(req.Sources) > 0 {
		b, err := json.Marshal(req.Sources)
		if err != nil {
			response.Error(c, 400, response.CodeInvalidParam, err.Error())
			return
		}
		kv["change_sources"] = string(b)
	}
	if req.WatchDirs != nil {
		kv["change_watch_dirs"] = strings.Join(trimAll(req.WatchDirs), ",")
	}
	if req.ExcludeGlobs != nil {
		kv["change_exclude_globs"] = strings.Join(trimAll(req.ExcludeGlobs), ",")
	}
	if req.Realtime != nil {
		kv["change_realtime"] = "true"
		if !*req.Realtime {
			kv["change_realtime"] = "false"
		}
	}
	if len(kv) == 0 {
		response.Error(c, 400, response.CodeInvalidParam, "没有需要保存的设置")
		return
	}
	for k, v := range kv {
		if err := h.DB.Save(&model.Setting{Key: k, Value: v, UpdatedAt: time.Now()}).Error; err != nil {
			response.Error(c, 500, response.CodeInternal, err.Error())
			return
		}
	}

	// Saved is not applied: sources hold their goroutines, so the watcher has to
	// be rebuilt. Report failure honestly instead of showing "saved" and leaving
	// the old collectors running.
	if h.Reload != nil {
		if err := h.Reload(); err != nil {
			response.Error(c, 500, response.CodeInternal, "设置已保存，但重建采集器失败："+err.Error()+"（重启面板后生效）")
			return
		}
	}
	response.OK(c, gin.H{"reloaded": h.Reload != nil})
}

// settingValue reads one panel setting, returning "" when absent.
func (h *ChangeHandler) settingValue(key string) string {
	if h.DB == nil {
		return ""
	}
	var s model.Setting
	if err := h.DB.Where("key = ?", key).First(&s).Error; err != nil {
		return ""
	}
	return s.Value
}

// csvField splits a comma separated setting, dropping blanks.
func csvField(v string) []string {
	out := []string{}
	for _, part := range strings.Split(v, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// trimAll drops blanks from a user-supplied list.
func trimAll(in []string) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		if p := strings.TrimSpace(v); p != "" {
			out = append(out, p)
		}
	}
	return out
}
