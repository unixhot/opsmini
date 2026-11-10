package v1

import (
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/drill"
	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/pkg/response"
)

// DrillHandler exposes the fault-injection drills.
//
// The point of the endpoint is not the injection itself but the answer it gives
// afterwards: "启动后系统发现了吗、处置了吗、恢复了吗". So List() joins each
// drill with the incidents created since it started — otherwise the operator has
// to eyeball two pages and guess the timing.
type DrillHandler struct {
	Mgr *drill.Manager
	DB  *gorm.DB
}

// NewDrillHandler wires the drill manager and the database (the DB is only used
// to answer "did an incident appear after the drill started").
func NewDrillHandler(mgr *drill.Manager, db *gorm.DB) *DrillHandler {
	return &DrillHandler{Mgr: mgr, DB: db}
}

type drillIncident struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Status  string `json:"status"`
	Mode    string `json:"mode"`
	Created string `json:"created_at"`
}

// drillView flattens the spec and the live status into one object.
//
// Embedding both Spec and Status would put two Kind fields at the same depth,
// and encoding/json silently drops ambiguous fields — the UI would get a card
// with no kind and no way to start anything. Explicit fields, no surprise.
type drillView struct {
	Kind       drill.Kind `json:"kind"`
	Risk       string     `json:"risk"`
	Detector   string     `json:"detector"`
	TTLSeconds int        `json:"ttl_seconds"`

	Active    bool       `json:"active"`
	StartedAt *time.Time `json:"started_at,omitempty"`
	StopAt    *time.Time `json:"stop_at,omitempty"`
	Note      string     `json:"note,omitempty"`
	Error     string     `json:"error,omitempty"`

	// Incidents found since StartedAt (only meaningful while active or right
	// after a restore).
	IncidentCount int             `json:"incident_count"`
	Incidents     []drillIncident `json:"incidents"`
	// Alerts fired since StartedAt. Metrics land here, not in the incident list:
	// a threshold breach produces an alert (and a notification), and only the
	// families covered by a trigger policy become an incident. Reporting both is
	// the only way "did the system notice" gets an honest answer.
	AlertCount int          `json:"alert_count"`
	Alerts     []drillAlert `json:"alerts"`
	// Available is false when this build lacks the capability (no cron service,
	// no docker); the UI greys the button instead of failing on click.
	Available bool `json:"available"`
}

type drillAlert struct {
	ID      uint   `json:"id"`
	Rule    string `json:"rule_name"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Status  string `json:"status"`
	Created string `json:"created_at"`
}

// List GET /drills
func (h *DrillHandler) List(c *gin.Context) {
	statuses := map[drill.Kind]drill.Status{}
	for _, s := range h.Mgr.List() {
		statuses[s.Kind] = s
	}
	out := make([]drillView, 0, len(statuses))
	for _, spec := range drill.Specs() {
		st := statuses[spec.Kind]
		v := drillView{
			Kind: spec.Kind, Risk: spec.Risk, Detector: spec.Detector, TTLSeconds: spec.TTLSeconds,
			Active: st.Active, StartedAt: st.StartedAt, StopAt: st.StopAt, Note: st.Note, Error: st.Error,
			Incidents: []drillIncident{}, Available: true,
		}
		if v.StartedAt != nil && h.DB != nil {
			var rows []model.Incident
			h.DB.Where("created_at >= ?", *v.StartedAt).
				Order("created_at desc").Limit(5).Find(&rows)
			v.Incidents = make([]drillIncident, 0, len(rows))
			for _, r := range rows {
				v.Incidents = append(v.Incidents, drillIncident{
					ID: r.ID, Title: r.Title, Status: r.Status, Mode: r.Mode,
					Created: r.CreatedAt.Format(time.RFC3339),
				})
			}
			v.IncidentCount = len(v.Incidents)

			var alerts []model.AlertEvent
			h.DB.Where("created_at >= ? AND kind = ?", *v.StartedAt, model.AlertKindAlert).
				Order("created_at desc").Limit(5).Find(&alerts)
			v.Alerts = make([]drillAlert, 0, len(alerts))
			for _, a := range alerts {
				v.Alerts = append(v.Alerts, drillAlert{
					ID: a.ID, Rule: a.RuleName, Level: a.Level, Message: a.Message,
					Status: a.Status, Created: a.CreatedAt.Format(time.RFC3339),
				})
			}
			v.AlertCount = len(v.Alerts)
		}
		out = append(out, v)
	}
	response.OK(c, gin.H{"drills": out, "checked_at": time.Now()})
}

type drillActionReq struct {
	// Confirm is required for guard-level drills (disk fill, docker stop).
	Confirm bool `json:"confirm"`
}

// Start POST /drills/:kind/start
func (h *DrillHandler) Start(c *gin.Context) {
	var req drillActionReq
	_ = c.ShouldBindJSON(&req)
	st, err := h.Mgr.Start(c.Request.Context(), drill.Kind(c.Param("kind")), req.Confirm)
	if err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	response.OK(c, st)
}

// Stop POST /drills/:kind/stop
func (h *DrillHandler) Stop(c *gin.Context) {
	st, err := h.Mgr.Stop(c.Request.Context(), drill.Kind(c.Param("kind")))
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, st)
}
