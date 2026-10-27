package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/aiagent"
	v1 "github.com/opsmini/opsmini/internal/api/v1"
	"github.com/opsmini/opsmini/internal/config"
	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/safety"
	"github.com/opsmini/opsmini/internal/sentinel"
	"github.com/opsmini/opsmini/internal/service"
	"github.com/opsmini/opsmini/internal/trigger"
)

// sentinelBundle groups the perception layer, the trigger engine and the HTTP
// handler so the router can start/stop them together.
type sentinelBundle struct {
	handler *v1.SentinelHandler
	manager *sentinel.Manager
	engine   *trigger.Engine
	safety   *safety.Manager
	metricCh chan model.Signal
}

// buildSentinel wires the v1.3.0 autonomous loop:
// collectors -> bus -> incidents -> trigger engine -> agent runtime.
func buildSentinel(
	cfg *config.Config,
	db *gorm.DB,
	settingSvc *service.SettingService,
	notifier *service.Notifier,
	ai aiAgentBundle,
	dataDir func() string,
	metricCh chan model.Signal,
) *sentinelBundle {
	sm := safety.New(db, dataDir, cfg.Server.Port)

	// Perception configuration, overridable from panel settings.
	sc := sentinel.DefaultConfig()
	sc.DataDir = dataDir()
	if v, _ := settingSvc.Get("sentinel_enabled"); v == "false" || v == "0" {
		sc.Enabled = false
	}
	if v, _ := settingSvc.Get("sentinel_learning"); v == "true" || v == "1" {
		sc.Learning = true
	}
	if v, _ := settingSvc.Get("sentinel_log_paths"); v != "" {
		sc.LogPaths = splitCSV(v)
	}
	if v, _ := settingSvc.Get("sentinel_web_roots"); v != "" {
		sc.WebRoots = splitCSV(v)
	}

	mgr := sentinel.New(sc, db)

	// Metric alerts from the existing engine feed the same bus.
	mgr.NewWithMetricHook(metricCh)

	eng := trigger.NewEngine(db, sm)
	eng.Learning = sc.Learning
	if err := eng.SeedDefaults(); err != nil {
		log.Printf("sentinel: seed policies: %v", err)
	}

	eng.Runner = &incidentRunner{
		agent:    ai.agent,
		store:    ai.store,
		db:       db,
		safety:   sm,
		dataDir:  dataDir,
		settingSvc: settingSvc,
	}
	eng.Notify = func(inc model.Incident, text string) {
		level := "warning"
		if inc.Severity == model.SeverityCritical {
			level = "critical"
		}
		notifier.Send(service.NotifyMessage{
			Channels:    "panel,email",
			Level:       level,
			PanelTitle:  "自主处置：" + inc.Title,
			PanelBody:   text,
			MailSubject: "[OpsMini] Agent: " + inc.Title,
			MailBody:    text,
			WebhookText: text,
		})
	}

	mgr.Agg.OnIncident(eng.OnIncident)

	handler := v1.NewSentinelHandler(db, mgr, eng, sm)
	handler.Setting = func(key, value string) error { return settingSvc.Set(map[string]string{key: value}) }

	// Retention windows survive restarts via the panel settings table.
	if raw, err := settingSvc.Get("sentinel_retention"); err == nil && strings.TrimSpace(raw) != "" {
		var rp sentinel.RetentionPolicy
		if err := json.Unmarshal([]byte(raw), &rp); err == nil {
			mgr.SetRetention(rp)
		} else {
			log.Printf("sentinel: invalid stored retention policy: %v", err)
		}
	}

	bundle := &sentinelBundle{
		handler: handler,
		manager: mgr,
		engine:  eng,
		safety:  sm,
	}
	bundle.metricCh = metricCh
	return bundle
}

// IncidentRunner adapts the AI Agent to the trigger engine.

type incidentRunner struct {
	agent      *aiagent.Agent
	store      *aiagent.Store
	db         *gorm.DB
	safety     *safety.Manager
	dataDir    func() string
	settingSvc *service.SettingService
}

// RunIncident executes one event-triggered run.
func (r *incidentRunner) RunIncident(ctx context.Context, req trigger.RunRequest) (trigger.RunResult, error) {
	out := trigger.RunResult{}
	if r.agent == nil {
		return out, fmt.Errorf("AI 模型未配置，事件只记录不分析")
	}
	title := fmt.Sprintf("%s · %s", req.Incident.Title, time.Now().Format("01-02 15:04"))
	start := time.Now()

	evidence := make([]string, 0, len(req.Signals))
	for _, s := range req.Signals {
		line := fmt.Sprintf("[%s/%s] %s（%s）", s.Kind, s.Severity, s.Message, s.ObservedAt.Format("15:04:05"))
		if s.Evidence != "" {
			line += " 证据：" + compactEvidence(s.Evidence)
		}
		evidence = append(evidence, line)
	}

	opt := aiagent.RunOptions{
		UserID:       aiagent.SystemUserID,
		Username:     "sentinel",
		RoleName:     model.RoleAdmin,
		Unattended:   true, // no human is watching: confirm-level tools are refused
		Title:        title,
		MaxSteps:     req.MaxSteps,
		Mode:         req.Mode,
		IncidentID:   req.Incident.ID,
		AllowedTools: req.AllowedTools,
		DeniedTools:  req.DeniedTools,
		MaxWrites:    req.MaxWrites,
		AutoTools:    autoToolsFor(req.Mode, req.Policy.AutoTools),
		Evidence:     evidence,
		OnAction: func(ev aiagent.ActionEvent) {
			r.logAction(req.Incident.ID, "", ev)
			if ev.Verdict == aiagent.ActionAllow && ev.Risk >= aiagent.RiskLowWrite {
				out.Writes++
			}
		},
	}

	res, err := r.agent.Send(ctx, opt, req.Prompt)
	if res != nil {
		out.SessionID = res.SessionID
		out.Steps = res.Steps
		out.Tools = res.Tools
		out.Skipped = res.Skipped
		out.Summary = res.Content
	}
	out.Writes = r.safety.WritesUsed(req.Incident.ID)

	// Reload the action log to fill verdicts that were written without a session
	// id (the session is only known after Send returns).
	r.db.Model(&model.ActionLog{}).Where("incident_id = ? AND session_id = ''", req.Incident.ID).
		Update("session_id", out.SessionID)

	// Archive the report next to the scheduled-task reports.
	if out.SessionID != "" {
		meta := aiagent.ReportMeta{
			TaskName:   req.Incident.Title,
			Prompt:     req.Prompt,
			Steps:      out.Steps,
			Tools:      out.Tools,
			Skipped:    out.Skipped,
			Duration:   time.Since(start),
			Unattended: true,
			Status:     "success",
		}
		if err != nil {
			meta.Status = "failed"
		}
		if md, berr := r.store.BuildReport(out.SessionID, meta); berr == nil {
			if art, serr := aiagent.ArchiveReport(r.dataDir(), "自主处置/"+sanitize(req.Incident.Title), out.SessionID, md, meta); serr == nil {
				out.ReportRel = art.RelHTML
			}
		}
	}
	return out, err
}

// logAction writes one entry of the decision chain.
func (r *incidentRunner) logAction(incidentID, sessionID string, ev aiagent.ActionEvent) {
	args, _ := json.Marshal(ev.Args)
	if len(args) > 4000 {
		args = args[:4000]
	}
	r.db.Create(&model.ActionLog{
		IncidentID: incidentID,
		SessionID:  sessionID,
		Tool:       ev.Name,
		Args:       string(args),
		Risk:       ev.Risk.String(),
		Verdict:    ev.Verdict,
		Rule:       ev.Rule,
		Result:     truncateStr(ev.Result, 2000),
		Reversible: ev.Reversible,
		DurationMS: ev.DurationMS,
		CreatedAt:  time.Now(),
	})
}

func autoToolsFor(mode, jsonList string) []string {
	if mode != model.ModeAuto || jsonList == "" {
		return nil
	}
	var out []string
	_ = json.Unmarshal([]byte(jsonList), &out)
	return out
}

// compactEvidence renders a stored evidence blob into one short line.
func compactEvidence(raw string) string {
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return truncateStr(raw, 200)
	}
	keys := []string{"path", "ip", "user", "bytes_per_min", "deviation", "holder_cmd",
		"template_top", "pid", "comm", "reason", "hits", "remote", "port", "off_hours"}
	var parts []string
	for _, k := range keys {
		if v, ok := m[k]; ok {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}
	}
	return truncateStr(strings.Join(parts, " "), 300)
}

func splitCSV(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func sanitize(s string) string {
	rep := strings.NewReplacer("/", "_", "\\", "_", ":", "_", " ", "_")
	return truncateStr(rep.Replace(s), 40)
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
