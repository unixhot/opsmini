package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/aiagent"
	"github.com/opsmini/opsmini/internal/aiagent/tools"
	v1 "github.com/opsmini/opsmini/internal/api/v1"
	"github.com/opsmini/opsmini/internal/change"
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
	handler   *v1.SentinelHandler
	manager   *sentinel.Manager
	engine    *trigger.Engine
	safety    *safety.Manager
	metricCh  chan model.Signal
	changes   *change.Watcher
	changeDB  *change.Store
	snapshots *change.Snapshots
	correlate *change.Correlator
	changeAPI *v1.ChangeHandler
	campaigns *sentinel.Campaigner
	shadow    *sentinel.ShadowReviewer
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

	// ---- v1.4 变更感知：采集 → 落库 → 扇出为 family=change 的 info 信号 ----
	ccfg := change.DefaultConfig()
	ccfg.Snapshot.Dir = filepath.Join(dataDir(), "changes")
	// Watch dirs are overridable from the panel (and by env var for tests /
	// containers where /etc is not the interesting tree).
	if v, _ := settingSvc.Get("change_watch_dirs"); strings.TrimSpace(v) != "" {
		ccfg.File.WatchDirs = splitCSV(v)
	}
	if v := os.Getenv("OPSMINI_CHANGE_WATCH_DIRS"); strings.TrimSpace(v) != "" {
		ccfg.File.WatchDirs = splitCSV(v)
	}
	if v, _ := settingSvc.Get("change_realtime"); v == "false" || v == "0" {
		ccfg.File.Realtime = false
	}
	changeStore, csErr := change.NewStore(db, ccfg.Quota.DiffTotalSize)
	snapshots := change.NewSnapshots(ccfg.Snapshot.Dir, ccfg.Snapshot.MaxPerDay, ccfg.Snapshot.MaxTotalSize)
	var correlator *change.Correlator
	var changeWatcher *change.Watcher
	if csErr != nil {
		log.Printf("sentinel: change store init failed: %v", csErr)
	} else {
		correlator = change.NewCorrelator(changeStore)
		// 把变更工具挂到已注册的 Agent 工具集上（store 在这里才构造出来）
		if ai.agent != nil {
			tools.RegisterChangeTools(ai.agent.Registry(), tools.Deps{Changes: changeStore})
		}
		// 变更复核：规则先判，判为可疑才把信号升到 high，让 change-review 策略接手。
		// 顺序很关键——必须在 Publish 之前改严重级，否则事件会以 info 建好，
		// 策略匹配不上，Agent 永远不会被唤醒。
		changeReviewer := change.NewReviewer(changeStore)
		changeWatcher, csErr = change.NewWatcher(ccfg, changeStore, snapshots, func(ev change.Event) {
			sig := changeSignal(ev)
			if rev, err := changeReviewer.Review(ev); err == nil && rev.Verdict == model.ChangeVerdictSuspicious {
				sig.Severity = model.SeverityHigh
				log.Printf("change: 可疑变更 %s（%.2f）→ 唤醒 Agent 分析", ev.Object.Key, rev.Score)
			}
			mgr.Bus.Publish(sig)
		})
		if csErr != nil {
			log.Printf("sentinel: change watcher init failed: %v", csErr)
			changeWatcher = nil
		}
	}

	eng.Runner = &incidentRunner{
		agent:      ai.agent,
		store:      ai.store,
		db:         db,
		safety:     sm,
		dataDir:    dataDir,
		settingSvc: settingSvc,
		correlate:  correlator,
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

	// 攻击链关联：把「WebShell 落地 → 可疑进程 → 异常外联」合成一次入侵来判断。
	// 只在事件层面汇总，不放大任何权限：预算、模式、可用工具一律不变。
	camp := sentinel.NewCampaigner(db)
	camp.Notify = func(c model.Campaign, list []model.Incident) {
		notifier.Send(service.NotifyMessage{
			Channels:    "panel,email",
			Level:       "critical",
			PanelTitle:  "疑似攻击链：" + c.Title,
			PanelBody:   c.Narrative,
			MailSubject: "[OpsMini] 疑似攻击链：" + c.Title,
			MailBody:    c.Narrative,
			WebhookText: c.Narrative,
		})
	}
	mgr.Agg.OnIncident(func(inc model.Incident, _ model.Signal) { camp.Attach(inc) })

	// 第二层关联：可疑变更作为攻击链的「持久化」环节。
	//
	// 只在三个条件同时成立时才并入：规则复核判为 suspicious、对象是能扛过重启
	// 的持久化机制（authorized_keys / cron / systemd / sudoers / ld.so.preload…）、
	// 且链上已经出现过 execution 或 c2。三条缺一条就不并入——「有人改了 nginx.conf」
	// 是运维事实，把它算成入侵环节会让攻击链本身失去可信度。
	camp.ChangeStage = func(inc model.Incident) (string, bool) {
		if inc.Family != model.FamilyChange || changeStore == nil || inc.Subject == "" {
			return "", false
		}
		ev, err := changeStore.LatestByObject(inc.Subject)
		if err != nil || ev.ID == "" {
			return "", false
		}
		verdict := ""
		if rev, err := changeStore.ReviewFor(ev.ID); err == nil && rev != nil {
			verdict = rev.Verdict
		}
		return change.ChainStage(ev.Kind, ev.ObjectKey, verdict)
	}

	// 影子评分：影子运行没有验证断言，只能在事后看「它当时想做的事还需不需要」。
	shadow := sentinel.NewShadowReviewer(db)

	handler := v1.NewSentinelHandler(db, mgr, eng, sm)
	handler.Campaigner = camp
	handler.Shadow = shadow
	var changeAPI *v1.ChangeHandler
	if changeStore != nil {
		changeAPI = v1.NewChangeHandler(db, changeStore, snapshots, changeWatcher, correlator)
	}
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
		handler:   handler,
		manager:   mgr,
		engine:    eng,
		safety:    sm,
		changes:   changeWatcher,
		changeDB:  changeStore,
		snapshots: snapshots,
		correlate: correlator,
		changeAPI: changeAPI,
		campaigns: camp,
		shadow:    shadow,
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
	correlate  *change.Correlator
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

	// v1.4：把"事件发生前 30 分钟内相关的系统变更"作为证据注进去。
	// 只注入元数据（路径/行数/字段名/归因/相关性），不给配置文件正文——
	// 正文是不可信输入，也是提示注入的天然载体，要看细节必须走 change_diff
	// 工具，那一步本身就是提高攻击成本。
	if r.correlate != nil {
		for _, rc := range r.correlate.Related(req.Incident, change.DefaultWindow) {
			evidence = append(evidence, "变更｜"+rc.Brief())
		}
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
				r.journalWrite(req.Incident.ID, ev)
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
// journaledBySafety are writes whose undo information is recorded by the safety
// layer (or the tool itself) with the real session id. Journaling them again here
// would produce a second, non-undoable row for the same action and make the rollback
// report claim it could not undo something it already undid.
var journaledBySafety = map[string]bool{
	"quarantine_file": true, "block_ip": true, "freeze_process": true, "truncate_log": true,
	"restore_quarantine": true, "restore_snapshot": true, "unblock_ip": true, "resume_process": true,
}

// journalWrite records a write that has no inverse. Rollback cannot undo it, but it
// must be able to *say so* — "3 项无法回滚" is the honest half of transaction
// semantics, and it is the half that tells a human where to look.
func (r *incidentRunner) journalWrite(incidentID string, ev aiagent.ActionEvent) {
	if r.safety == nil || journaledBySafety[ev.Name] {
		return
	}
	target := ""
	if v, ok := ev.Args["path"].(string); ok {
		target = v
	} else if v, ok := ev.Args["command"].(string); ok {
		target = truncateStr(v, 200)
	} else if v, ok := ev.Args["unit"].(string); ok {
		target = v
	}
	r.safety.Journal(incidentID, ev.Name, target, 0, false)
}

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

// changeSignal renders a change record as an info-level signal so it shows up in
// the event centre and can be correlated. Severity is always info and family is
// always "change": the trigger engine refuses to let this family write.
func changeSignal(ev change.Event) model.Signal {
	added, removed := 0, 0
	if ev.Diff != nil {
		added, removed = ev.Diff.Added, ev.Diff.Removed
	}
	payload := map[string]any{
		"kind": ev.Kind, "object": ev.Object.Key,
		"added": added, "removed": removed,
		"actor": ev.Actor.User, "src_ip": ev.Actor.IP,
		"how": ev.Actor.How, "confidence": ev.Actor.Confidence,
	}
	raw, _ := json.Marshal(payload)
	return model.Signal{
		ID:          ev.ID(),
		Kind:        model.SignalKindChange,
		Source:      ev.Source,
		Subject:     ev.Object.Key,
		Severity:    model.SeverityInfo,
		Fingerprint: ev.Fingerprint(),
		Family:      model.FamilyChange,
		DedupKey:    ev.DedupKey(),
		Message:     ev.Summary(),
		Evidence:    string(raw),
		Tags:        truncateStr(strings.Join(ev.Object.Labels, ","), 240),
		ObservedAt:  ev.At,
		CreatedAt:   time.Now(),
	}
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

// startBackground runs the parts of the loop that need a clock rather than an event:
// attack-chain closing and shadow scoring. Both are bookkeeping — they never touch
// the host and never call the model.
func (b *sentinelBundle) startBackground(ctx context.Context) {
	if b.campaigns != nil {
		// 启动时先回填：升级前或停机期间产生的事件也要能进链，
		// 否则面板会一直是空的，用户分不清「没有入侵」和「功能没生效」。
		go func() {
			if n := b.campaigns.Backfill(24 * time.Hour); n > 0 {
				log.Printf("sentinel: campaigns backfilled from existing incidents: %d", n)
			}
		}()
		go func() {
			t := time.NewTicker(30 * time.Minute)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-t.C:
					if n := b.campaigns.Sweep(12 * time.Hour); n > 0 {
						log.Printf("sentinel: campaigns closed: %d", n)
					}
					b.campaigns.Backfill(24 * time.Hour)
				}
			}
		}()
	}
	if b.shadow != nil {
		b.shadow.Start(ctx, 30*time.Minute)
	}
}
