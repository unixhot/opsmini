package v1

import (
	"encoding/json"
	"fmt"
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
	// Campaigner and Shadow are optional: they are only wired when the perception
	// layer is active, and every handler guards on nil.
	Campaigner *sentinel.Campaigner
	Shadow     *sentinel.ShadowReviewer
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
	// Zero-fill every known state so the filter chips always render the same set
	// of numbers instead of flickering between 0 and blank.
	for _, st := range []string{model.IncidentOpen, model.IncidentTriggered, model.IncidentRunning,
		model.IncidentResolved, model.IncidentIgnored, model.IncidentTakenOver, model.IncidentFailed} {
		statusMap[st] = int64(0)
	}
	var total int64
	h.DB.Model(&model.Incident{}).Count(&total)
	for _, r := range byStatus {
		statusMap[r.Status] = r.N
	}
	out["incidents_open"] = open
	out["incidents_unresolved"] = unresolved
	out["incidents_total"] = total
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
	if cid := c.Query("campaign"); cid != "" {
		q = q.Where("campaign_id = ?", cid)
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
	// Campaign titles in one extra query, so the list can badge "属于攻击链" without
	// the client fetching every incident's detail.
	titles := map[string]string{}
	var cids []string
	for _, i := range list {
		if i.CampaignID != "" {
			cids = append(cids, i.CampaignID)
		}
	}
	if len(cids) > 0 {
		var camps []model.Campaign
		h.DB.Select("id", "title").Where("id IN ?", cids).Find(&camps)
		for _, cp := range camps {
			titles[cp.ID] = cp.Title
		}
	}
	response.OK(c, gin.H{"list": list, "campaign_titles": titles, "total": total, "page": page, "page_size": size})
}

// Campaigns GET /sentinel/campaigns
//
// Lists correlated attack chains. Only chains with more than one incident are worth
// showing — a "chain" of one is just an incident, and listing those would defeat the
// purpose of the view.
func (h *SentinelHandler) Campaigns(c *gin.Context) {
	page, size, off := pageParams(c)
	q := h.DB.Model(&model.Campaign{})
	if st := c.Query("status"); st != "" && st != "all" {
		q = q.Where("status = ?", st)
	}
	var total int64
	q.Count(&total)
	var list []model.Campaign
	if err := q.Order("last_seen DESC").Offset(off).Limit(size).Find(&list).Error; err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	// Per-campaign incident counts in one grouped query (the stored counter can lag
	// behind incidents that were attached while a notification was in flight).
	type row struct {
		CampaignID string
		N          int64
	}
	var rows []row
	h.DB.Model(&model.Incident{}).Select("campaign_id, count(*) as n").
		Where("campaign_id <> ''").Group("campaign_id").Scan(&rows)
	counts := map[string]int64{}
	for _, r := range rows {
		counts[r.CampaignID] = r.N
	}

	// Stage provenance: which collector put each stage into the chain. Derived from
	// the incidents rather than stored on the campaign, so existing chains answer
	// the question immediately and no migration is needed.
	type occ struct {
		CampaignID string
		Family     string
		N          int64
	}
	var occs []occ
	if ids := campaignIDs(list); len(ids) > 0 {
		h.DB.Model(&model.Incident{}).Select("campaign_id, family, count(*) as n").
			Where("campaign_id IN ?", ids).Group("campaign_id, family").Scan(&occs)
	}
	sources := map[string]map[string][]string{}
	for _, o := range occs {
		stage, ok := sentinel.StageOfFamily(o.Family)
		if !ok {
			continue
		}
		if sources[o.CampaignID] == nil {
			sources[o.CampaignID] = map[string][]string{}
		}
		sources[o.CampaignID][stage] = append(sources[o.CampaignID][stage], o.Family)
	}

	response.OK(c, gin.H{"list": list, "counts": counts, "sources": sources,
		"total": total, "page": page, "page_size": size})
}

// campaignIDs collects the ids of one page of campaigns.
func campaignIDs(list []model.Campaign) []string {
	out := make([]string, 0, len(list))
	for _, c := range list {
		out = append(out, c.ID)
	}
	return out
}

// Campaign GET /sentinel/campaigns/:id
func (h *SentinelHandler) Campaign(c *gin.Context) {
	var camp model.Campaign
	if err := h.DB.First(&camp, "id = ?", c.Param("id")).Error; err != nil {
		response.Error(c, 404, response.CodeNotFound, "攻击链不存在")
		return
	}
	var incidents []model.Incident
	h.DB.Where("campaign_id = ?", camp.ID).Order("first_seen").Find(&incidents)
	response.OK(c, gin.H{"campaign": camp, "incidents": incidents})
}

// Rollback POST /sentinel/incidents/:id/rollback
//
// Manual undo for a run that already finished. The engine rolls back automatically
// when verification fails; this endpoint exists for the cases it cannot judge — an
// operator looking at a resolved incident and disagreeing with what it changed.
func (h *SentinelHandler) Rollback(c *gin.Context) {
	id := c.Param("id")
	var inc model.Incident
	if err := h.DB.First(&inc, "id = ?", id).Error; err != nil {
		response.Error(c, 404, response.CodeNotFound, "事件不存在")
		return
	}
	if h.Safety == nil {
		response.Error(c, 500, response.CodeInternal, "安全层不可用")
		return
	}
	var sessions []string
	h.DB.Model(&model.AgentRun{}).Where("incident_id = ? AND session_id <> ?", id, "").
		Pluck("session_id", &sessions)
	sessions = append(sessions, id)

	steps := h.Safety.RollbackSessions(sessions)
	if len(steps) == 0 {
		response.OK(c, gin.H{"steps": []any{}, "message": "没有可回滚的操作"})
		return
	}
	undone, skipped, failedCount := 0, 0, 0
	for _, s := range steps {
		switch {
		case s.OK:
			undone++
		case s.Skipped:
			skipped++
		default:
			failedCount++
		}
	}
	response.OK(c, gin.H{
		"steps": steps, "undone": undone, "skipped": skipped, "failed": failedCount,
		"message": fmt.Sprintf("已撤销 %d 项，%d 项不可撤销，%d 项失败", undone, skipped, failedCount),
	})
}

// ShadowReview POST /sentinel/incidents/:id/shadow-review
//
// Re-scores a shadow run on demand. Automatic reviews run on a timer; this lets an
// operator refresh the judgement right after the world changed.
func (h *SentinelHandler) ShadowReview(c *gin.Context) {
	if h.Shadow == nil {
		response.Error(c, 500, response.CodeInternal, "影子评审不可用")
		return
	}
	rev, err := h.Shadow.Review(c.Param("id"))
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, gin.H{"review": rev})
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

	// Journal + campaign ride along with the detail: the transaction view ("what did
	// this run change, and can it be undone") is useless anywhere else.
	var journal []model.ActionJournal
	if h.Safety != nil {
		sessions := []string{}
		for _, r := range runs {
			if r.SessionID != "" {
				sessions = append(sessions, r.SessionID)
			}
		}
		// Non-undoable writes are journaled under the incident id.
		sessions = append(sessions, inc.ID)
		journal = h.Safety.JournalFor(sessions)
	}
	var campaign *model.Campaign
	if inc.CampaignID != "" {
		var camp model.Campaign
		if err := h.DB.First(&camp, "id = ?", inc.CampaignID).Error; err == nil {
			campaign = &camp
		}
	}
	var shadow *model.ShadowReview
	h.DB.Where("incident_id = ?", id).Order("id DESC").Limit(1).Find(&shadow)

	response.OK(c, gin.H{
		"incident": inc,
		"signals":  signals,
		"actions":  actions,
		"runs":     runs,
		"journal":  journal,
		"campaign": campaign,
		"shadow":   shadow,
	})
}

// Members GET /sentinel/incidents/:id/members
//
// The stored member list on the incident row is capped (memberCap) so one row cannot
// grow without bound while an attack keeps adding addresses. That cap is invisible in
// the panel until someone wants the whole list, which is exactly when it matters — so
// rebuild it from the signals that were folded in.
func (h *SentinelHandler) Members(c *gin.Context) {
	id := c.Param("id")
	var exists int64
	h.DB.Model(&model.Incident{}).Where("id = ?", id).Count(&exists)
	if exists == 0 {
		response.Error(c, 404, response.CodeNotFound, "事件不存在")
		return
	}
	// Hard ceiling: a brute force from 200k addresses must not turn one click into a
	// multi-megabyte response. The flag tells the UI it is looking at a subset.
	const maxReturn = 1000
	var total int64
	h.DB.Model(&model.Signal{}).Where("incident_id = ? AND subject <> ''", id).
		Distinct("subject").Count(&total)
	var subjects []string
	h.DB.Model(&model.Signal{}).Where("incident_id = ? AND subject <> ''", id).
		Distinct().Order("subject").Limit(maxReturn).Pluck("subject", &subjects)
	response.OK(c, gin.H{
		"members":   subjects,
		"total":     total,
		"truncated": int64(len(subjects)) < total,
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

// Feedback POST /sentinel/incidents/:id/feedback
//
// A human verdict on the incident ("this was right" / "this was noise"). Ignore and
// takeover already imply false_positive; this endpoint exists for the opposite case
// and for shadow runs, where nothing was executed and the only thing a human can
// judge is whether the analysis was worth reading. The rows land in signal_feedbacks
// so a later tuner can use them without re-reading the incident table.
func (h *SentinelHandler) Feedback(c *gin.Context) {
	id := c.Param("id")
	var inc model.Incident
	if err := h.DB.First(&inc, "id = ?", id).Error; err != nil {
		response.Error(c, 404, response.CodeNotFound, "事件不存在")
		return
	}
	var req struct {
		Verdict string `json:"verdict"`
		Note    string `json:"note"`
		By      string `json:"by"`
	}
	_ = c.ShouldBindJSON(&req)
	verdict := strings.TrimSpace(req.Verdict)
	if verdict != "true_positive" && verdict != "false_positive" {
		response.Error(c, 400, response.CodeInvalidParam, "verdict 只能是 true_positive 或 false_positive")
		return
	}
	if req.By == "" {
		// The auth middleware only puts the numeric user id on the context, so look
		// the name up: a feedback row without an author is not auditable.
		if uid, ok := c.Get("userID"); ok {
			var u model.User
			if err := h.DB.Select("username").First(&u, "id = ?", uid).Error; err == nil {
				req.By = u.Username
			}
		}
	}
	var sig model.Signal
	_ = h.DB.Where("incident_id = ?", id).Order("observed_at").First(&sig).Error
	h.DB.Create(&model.SignalFeedback{
		SignalID: sig.ID, Fingerprint: sig.Fingerprint,
		Verdict: verdict, Note: req.Note, By: req.By,
	})
	response.OK(c, gin.H{"status": "recorded"})
}

// Takeover POST /sentinel/incidents/:id/takeover
func (h *SentinelHandler) Takeover(c *gin.Context) {
	id := c.Param("id")
	var inc model.Incident
	if err := h.DB.First(&inc, "id = ?", id).Error; err != nil {
		response.Error(c, 404, response.CodeNotFound, "事件不存在")
		return
	}
	var req struct {
		By string `json:"by"`
	}
	_ = c.ShouldBindJSON(&req)
	by := strings.TrimSpace(req.By)
	if by == "" {
		// Same reason as the feedback endpoint: the middleware only puts the numeric
		// id on the context, and a handover without a name is not auditable.
		if uid, ok := c.Get("userID"); ok {
			var u model.User
			if err := h.DB.Select("username").First(&u, "id = ?", uid).Error; err == nil {
				by = u.Username
			}
		}
	}
	upd := map[string]any{"status": model.IncidentTakenOver, "ignored_by": by, "updated_at": time.Now()}
	if inc.PolicyName != "" {
		// Keep the policy name for history; takeover does not rewrite what happened.
		upd["policy_name"] = inc.PolicyName
	}
	if err := h.DB.Model(&model.Incident{}).Where("id = ?", id).Updates(upd).Error; err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"status": model.IncidentTakenOver, "by": by})
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

// Reset POST /sentinel/reset  {"scope":"all|events","dry_run":true,"confirm":"RESET"}
//
// This is the "clear the event history" button: it ignores retention windows
// and the open-incident protection on purpose, so it requires an explicit
// confirm token. Scope "all" also releases firewall bans before their records
// disappear, otherwise the machine would stay blocked with no trace in the UI.
func (h *SentinelHandler) Reset(c *gin.Context) {
	var req struct {
		Scope   string `json:"scope"`
		DryRun  bool   `json:"dry_run"`
		Confirm string `json:"confirm"`
	}
	_ = c.ShouldBindJSON(&req)
	if req.Scope == "" {
		req.Scope = "all"
	}
	if !req.DryRun && req.Confirm != "RESET" {
		response.Error(c, 400, response.CodeInvalidParam, "需要确认：confirm 必须为 RESET")
		return
	}
	if !req.DryRun && req.Scope == "all" && h.Safety != nil {
		var active []model.BlockedIP
		h.DB.Where("released_at IS NULL").Find(&active)
		for _, b := range active {
			_ = h.Safety.UnblockIP(b.IP)
		}
	}
	res, err := h.Manager.Reset(req.Scope, req.DryRun)
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
