package v1

import (
	"strconv"
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
}

// NewSentinelHandler constructor.
func NewSentinelHandler(db *gorm.DB, m *sentinel.Manager, e *trigger.Engine, s *safety.Manager) *SentinelHandler {
	return &SentinelHandler{DB: db, Manager: m, Engine: e, Safety: s}
}

// Stats GET /sentinel/stats
func (h *SentinelHandler) Stats(c *gin.Context) {
	out := gin.H{"perception": h.Manager.Stats()}
	var sig24, incOpen, incToday, actions int64
	cut := time.Now().Add(-24 * time.Hour)
	h.DB.Model(&model.Signal{}).Where("observed_at > ?", cut).Count(&sig24)
	h.DB.Model(&model.Incident{}).Where("status IN ?", []string{
		model.IncidentOpen, model.IncidentTriggered, model.IncidentRunning}).Count(&incOpen)
	h.DB.Model(&model.Incident{}).Where("created_at > ?", cut).Count(&incToday)
	h.DB.Model(&model.ActionLog{}).Where("created_at > ?", cut).Count(&actions)
	out["signals_24h"] = sig24
	out["incidents_open"] = incOpen
	out["incidents_24h"] = incToday
	out["actions_24h"] = actions
	out["paused"] = h.Safety.Paused()
	out["learning"] = h.Manager.Config().Learning
	response.OK(c, out)
}

// Signals GET /sentinel/signals
func (h *SentinelHandler) Signals(c *gin.Context) {
	limit := atoiDefault(c.Query("limit"), 100)
	if limit > 500 {
		limit = 500
	}
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
	var list []model.Signal
	if err := q.Order("observed_at DESC").Limit(limit).Find(&list).Error; err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
}

// Incidents GET /sentinel/incidents
func (h *SentinelHandler) Incidents(c *gin.Context) {
	limit := atoiDefault(c.Query("limit"), 100)
	q := h.DB.Model(&model.Incident{})
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
	}
	var list []model.Incident
	if err := q.Order("last_seen DESC").Limit(limit).Find(&list).Error; err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, list)
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

// Pause POST /sentinel/pause   {"paused": true}
func (h *SentinelHandler) Pause(c *gin.Context) {
	var req struct {
		Paused bool `json:"paused"`
	}
	_ = c.ShouldBindJSON(&req)
	h.Safety.Pause(req.Paused)
	response.OK(c, gin.H{"paused": req.Paused})
}

// Actions GET /sentinel/actions
func (h *SentinelHandler) Actions(c *gin.Context) {
	limit := atoiDefault(c.Query("limit"), 100)
	var list []model.ActionLog
	h.DB.Order("id DESC").Limit(limit).Find(&list)
	response.OK(c, list)
}

// Quarantine GET /sentinel/quarantine
func (h *SentinelHandler) Quarantine(c *gin.Context) {
	var list []model.QuarantineItem
	h.DB.Order("id DESC").Limit(200).Find(&list)
	response.OK(c, list)
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

// BlockedIPs GET /sentinel/blocks
func (h *SentinelHandler) BlockedIPs(c *gin.Context) {
	var list []model.BlockedIP
	h.DB.Order("id DESC").Limit(200).Find(&list)
	response.OK(c, list)
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
