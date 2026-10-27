package v1

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/pkg/response"
	"github.com/opsmini/opsmini/internal/safety"
	"github.com/opsmini/opsmini/internal/sentinel"
	"github.com/opsmini/opsmini/internal/trigger"
)

// SentinelHandler exposes the perception layer, the incident list, the trigger
// policies and the safety primitives to the panel UI.
type SentinelHandler struct {
	DB      *gorm.DB
	Manager *sentinel.Manager
	Engine  *trigger.Engine
	Safety  *safety.Manager
	// Setting persists small pieces of configuration (retention windows) in the
	// panel settings table so a restart does not reset them.
	Setting func(key string, value string) error
}

// NewSentinelHandler constructor.
func NewSentinelHandler(db *gorm.DB, m *sentinel.Manager, e *trigger.Engine, s *safety.Manager) *SentinelHandler {
	return &SentinelHandler{DB: db, Manager: m, Engine: e, Safety: s}
}

// responseTools are the reversible primitives the safety layer provides. A
// policy lists a subset of these in allow_tools / auto_tools.
var responseTools = []string{
	"quarantine_file", "restore_quarantine",
	"freeze_process", "resume_process",
	"snapshot_path", "restore_snapshot",
	"block_ip", "unblock_ip",
	"truncate_log",
}

// pageParams reads page/page_size with sane bounds.
func pageParams(c *gin.Context) (page, size, offset int) {
	page = atoiDefault(c.Query("page"), 1)
	size = atoiDefault(c.Query("page_size"), 20)
	if size > 200 {
		size = 200
	}
	if page > 10000 {
		page = 10000
	}
	return page, size, (page - 1) * size
}

// Stats GET /sentinel/stats
func (h *SentinelHandler) Stats(c *gin.Context) {
	out := gin.H{"perception": h.Manager.Stats()}
	var sig24, incToday, actions int64
	cut := time.Now().Add(-24 * time.Hour)
	h.DB.Model(&model.Signal{}).Where("observed_at > ?", cut).Count(&sig24)
	h.DB.Model(&model.Incident{}).Where("created_at > ?", cut).Count(&incToday)
	h.DB.Model(&model.ActionLog{}).Where("created_at > ?", cut).Count(&actions)
	out["signals_24h"] = sig24
	out["incidents_24h"] = incToday
	out["actions_24h"] = actions
	out["paused"] = h.Safety.Paused()
	out["learning"] = h.Manager.Config().Learning

	// Per-status counts so the chips above the incident table can show numbers.
	openStates := []string{model.IncidentOpen, model.IncidentTriggered, model.IncidentRunning}
	var open, unresolved int64
	h.DB.Model(&model.Incident{}).Where("status IN ?", openStates).Count(&open)
	h.DB.Model(&model.Incident{}).Where("status IN ?",
		append(openStates, model.IncidentFailed)).Count(&unresolved)
	var byStatus []struct {
		Status string
		N      int64
	}
	h.DB.Model(&model.Incident{}).Select("status, count(*) as n").Group("status").Scan(&byStatus)
	statusMap := gin.H{}
	for _, r := range byStatus {
		statusMap[r.Status] = r.N
	}
	out["incidents_open"] = open
	out["incidents_unresolved"] = unresolved
	out["by_status"] = statusMap
	response.OK(c, out)
}

// Facets GET /sentinel/facets
//
// Everything the UI needs to build filters and the policy editor: incident
// counts per scenario/severity plus the vocabulary (families, tools, modes).
func (h *SentinelHandler) Facets(c *gin.Context) {
	type countRow struct {
		Key   string
		Total int64
		Open  int64
	}
	var fam []countRow
	h.DB.Model(&model.Incident{}).
		Select("family as key, count(*) as total, sum(case when status in ('open','triggered','running') then 1 else 0 end) as open").
		Group("family").Order("total desc").Scan(&fam)

	var sev []countRow
	h.DB.Model(&model.Incident{}).Select("severity as key, count(*) as total, 0 as open").
		Group("severity").Scan(&sev)

	families := make([]gin.H, 0, len(fam))
	for _, r := range fam {
		if r.Key == "" {
			continue
		}
		families = append(families, gin.H{"key": r.Key, "total": r.Total, "open": r.Open})
	}
	severities := make([]gin.H, 0, len(sev))
	for _, r := range sev {
		if r.Key == "" {
			continue
		}
		severities = append(severities, gin.H{"key": r.Key, "total": r.Total})
	}
	response.OK(c, gin.H{
		"families":       families,
		"severities":     severities,
		"known_families": trigger.KnownFamilies(),
		"response_tools": responseTools,
		"modes":          []string{model.ModeShadow, model.ModeAssist, model.ModeAuto},
	})
}

// Signals GET /sentinel/signals?page=&page_size=&kind=&family=&severity=&q=
func (h *SentinelHandler) Signals(c *gin.Context) {
	page, size, off := pageParams(c)
	q := h.DB.Model(&model.Signal{})
	if k := c.Query("kind"); k != "" {
		q = q.Where("kind = ?", k)
	}
	if s := c.Query("severity"); s != "" {
		q = q.Where("severity = ?", s)
	}
	if f := c.Query("family"); f != "" {
		q = q.Where("family = ?", f)
	}
	if kw := strings.TrimSpace(c.Query("q")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("subject LIKE ? OR message LIKE ?", like, like)
	}
	var total int64
	q.Count(&total)
	var list []model.Signal
	if err := q.Order("observed_at DESC").Offset(off).Limit(size).Find(&list).Error; err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": size})
}

// Incidents GET /sentinel/incidents?page=&page_size=&status=&family=&severity=&q=
func (h *SentinelHandler) Incidents(c *gin.Context) {
	page, size, off := pageParams(c)
	q := h.DB.Model(&model.Incident{})
	if s := c.Query("status"); s != "" {
		if s == "unresolved" {
			// Everything a human still has to look at.
			q = q.Where("status IN ?", []string{
				model.IncidentOpen, model.IncidentTriggered,
				model.IncidentRunning, model.IncidentFailed})
		} else if s != "all" {
			q = q.Where("status = ?", s)
		}
	}
	if f := c.Query("family"); f != "" {
		q = q.Where("family = ?", f)
	}
	if v := c.Query("severity"); v != "" {
		q = q.Where("severity = ?", v)
	}
	if kw := strings.TrimSpace(c.Query("q")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("title LIKE ? OR subject LIKE ? OR summary LIKE ?", like, like, like)
	}
	var total int64
	q.Count(&total)
	var list []model.Incident
	if err := q.Order("last_seen DESC").Offset(off).Limit(size).Find(&list).Error; err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": size})
}

// Incident GET /sentinel/incidents/:id
func (h *SentinelHandler) Incident(c *gin.Context) {
	id := c.Param("id")
	var inc model.Incident
	if err := h.DB.First(&inc, "id = ?", id).Error; err != nil {
		response.Error(c, 404, response.CodeNotFound, "事件不存在")
		return
	}
	var signals []model.Signal
	h.DB.Where("incident_id = ?", id).Order("observed_at").Find(&signals)
	var actions []model.ActionLog
	h.DB.Where("incident_id = ?", id).Order("id").Find(&actions)
	var runs []model.AgentRun
	h.DB.Where("incident_id = ?", id).Order("id").Find(&runs)

	response.OK(c, gin.H{
		"incident": inc,
		"signals":  signals,
		"actions":  actions,
		"runs":     runs,
	})
}

// Ignore POST /sentinel/incidents/:id/ignore
func (h *SentinelHandler) Ignore(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Note   string `json:"note"`
		Signal string `json:"signal_id"`
		By     string `json:"by"`
	}
	_ = c.ShouldBindJSON(&req)
	var inc model.Incident
	if err := h.DB.First(&inc, "id = ?", id).Error; err != nil {
		response.Error(c, 404, response.CodeNotFound, "事件不存在")
		return
	}
	h.DB.Model(&model.Incident{}).Where("id = ?", id).Updates(map[string]any{
		"status": model.IncidentIgnored, "ignored_by": req.By, "ignore_note": req.Note,
	})
	// Teach the tuner: the first signal of the incident was a false positive.
	var sig model.Signal
	if err := h.DB.Where("incident_id = ?", id).Order("observed_at").First(&sig).Error; err == nil {
		h.DB.Create(&model.SignalFeedback{
			SignalID: sig.ID, Fingerprint: sig.Fingerprint,
			Verdict: "false_positive", Note: req.Note, By: req.By,
		})
	}
	response.OK(c, gin.H{"status": "ignored"})
}

// Takeover POST /sentinel/incidents/:id/takeover
func (h *SentinelHandler) Takeover(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		By string `json:"by"`
	}
	_ = c.ShouldBindJSON(&req)
	h.DB.Model(&model.Incident{}).Where("id = ?", id).
		Updates(map[string]any{"status": model.IncidentTakenOver, "ignored_by": req.By})
	response.OK(c, gin.H{"status": "taken_over"})
}

// Policies GET /sentinel/policies
func (h *SentinelHandler) Policies(c *gin.Context) {
	list, err := h.Engine.List()
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// CreatePolicy POST /sentinel/policies
func (h *SentinelHandler) CreatePolicy(c *gin.Context) {
	var in model.TriggerPolicy
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	p, err := h.Engine.Create(in)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, p)
}

// UpdatePolicy PUT /sentinel/policies/:id
func (h *SentinelHandler) UpdatePolicy(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var in model.TriggerPolicy
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	p, err := h.Engine.Update(uint(id), in)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, p)
}

// DeletePolicy DELETE /sentinel/policies/:id
func (h *SentinelHandler) DeletePolicy(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.Engine.Delete(uint(id)); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Pause POST /sentinel/pause   {"paused": true}
func (h *SentinelHandler) Pause(c *gin.Context) {
	var req struct {
		Paused bool `json:"paused"`
	}
	_ = c.ShouldBindJSON(&req)
	h.Safety.Pause(req.Paused)
	response.OK(c, gin.H{"paused": req.Paused})
}

// Actions GET /sentinel/actions?page=&page_size=&tool=&verdict=
func (h *SentinelHandler) Actions(c *gin.Context) {
	page, size, off := pageParams(c)
	q := h.DB.Model(&model.ActionLog{})
	if t := c.Query("tool"); t != "" {
		q = q.Where("tool = ?", t)
	}
	if v := c.Query("verdict"); v != "" {
		q = q.Where("verdict = ?", v)
	}
	if kw := strings.TrimSpace(c.Query("q")); kw != "" {
		like := "%" + kw + "%"
		q = q.Where("tool LIKE ? OR result LIKE ?", like, like)
	}
	var total int64
	q.Count(&total)
	var list []model.ActionLog
	if err := q.Order("id DESC").Offset(off).Limit(size).Find(&list).Error; err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": size})
}

// Quarantine GET /sentinel/quarantine?page=&page_size=
func (h *SentinelHandler) Quarantine(c *gin.Context) {
	page, size, off := pageParams(c)
	q := h.DB.Model(&model.QuarantineItem{})
	var total int64
	q.Count(&total)
	var list []model.QuarantineItem
	q.Order("id DESC").Offset(off).Limit(size).Find(&list)
	response.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": size})
}

// RestoreQuarantine POST /sentinel/quarantine/:id/restore
func (h *SentinelHandler) RestoreQuarantine(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	item, err := h.Safety.RestoreQuarantine(uint(id))
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, item)
}

// DeleteQuarantine DELETE /sentinel/quarantine/:id
func (h *SentinelHandler) DeleteQuarantine(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if err := h.DB.Delete(&model.QuarantineItem{}, id).Error; err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// BlockedIPs GET /sentinel/blocks?page=&page_size=
func (h *SentinelHandler) BlockedIPs(c *gin.Context) {
	page, size, off := pageParams(c)
	q := h.DB.Model(&model.BlockedIP{})
	if c.Query("active") == "1" {
		q = q.Where("released_at IS NULL AND expires_at > ?", time.Now())
	}
	var total int64
	q.Count(&total)
	var list []model.BlockedIP
	q.Order("id DESC").Offset(off).Limit(size).Find(&list)
	response.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": size})
}

// UnblockIP DELETE /sentinel/blocks/:ip
func (h *SentinelHandler) UnblockIP(c *gin.Context) {
	ip := c.Param("ip")
	if err := h.Safety.UnblockIP(ip); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"ip": ip, "released": true})
}

// Retention GET /sentinel/retention
func (h *SentinelHandler) Retention(c *gin.Context) {
	response.OK(c, gin.H{
		"policy": h.Manager.Retention,
		"stats":  h.Manager.RetentionStats(),
	})
}

// UpdateRetention PUT /sentinel/retention
func (h *SentinelHandler) UpdateRetention(c *gin.Context) {
	var in sentinel.RetentionPolicy
	if err := c.ShouldBindJSON(&in); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	h.Manager.SetRetention(in)
	if h.Setting != nil {
		if b, err := json.Marshal(h.Manager.Retention); err == nil {
			_ = h.Setting("sentinel_retention", string(b))
		}
	}
	response.OK(c, gin.H{"policy": h.Manager.Retention})
}

// PurgeRetention POST /sentinel/retention/purge  {"what":"all","dry_run":true}
func (h *SentinelHandler) PurgeRetention(c *gin.Context) {
	var req struct {
		What   string `json:"what"`
		DryRun bool   `json:"dry_run"`
	}
	_ = c.ShouldBindJSON(&req)
	res, err := h.Manager.Purge(req.What, req.DryRun)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, res)
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return def
	}
	return n
}
