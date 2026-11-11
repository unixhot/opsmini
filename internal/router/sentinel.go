package router

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
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
	// 降噪两项：维护窗口与每日触发上限。存在设置里，改完无需重启
	// （Engine 每次运行都读自己的字段，这里只在启动时载入一次初始值）。
	if v, _ := settingSvc.Get("trigger_maintenance_start"); v != "" {
		eng.MaintenanceStart = strings.TrimSpace(v)
	}
	if v, _ := settingSvc.Get("trigger_maintenance_end"); v != "" {
		eng.MaintenanceEnd = strings.TrimSpace(v)
	}
	if v, _ := settingSvc.Get("trigger_daily_limit"); v != "" {
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			eng.DailyLimit = n
		}
	}
	if err := eng.SeedDefaults(); err != nil {
		log.Printf("sentinel: seed policies: %v", err)
	}

	// ---- v1.4 变更感知：采集 → 落库 → 扇出为 family=change 的 info 信号 ----
	// 每次重建都从设置重新读一遍：采集源开关和监听目录都允许在界面上改，
	// 改完需要重建 watcher（各源在 Start 时按开关起 goroutine，运行中关不掉）。
	buildChangeConfig := func() *change.Config {
		c := change.DefaultConfig()
		c.Snapshot.Dir = filepath.Join(dataDir(), "changes")
		applyChangeSources(c, settingSvc)
		// Watch dirs are overridable from the panel (and by env var for tests /
		// containers where /etc is not the interesting tree).
		if v, _ := settingSvc.Get("change_watch_dirs"); strings.TrimSpace(v) != "" {
			c.File.WatchDirs = splitCSV(v)
		}
		if v := os.Getenv("OPSMINI_CHANGE_WATCH_DIRS"); strings.TrimSpace(v) != "" {
			c.File.WatchDirs = splitCSV(v)
		}
		if v, _ := settingSvc.Get("change_realtime"); v == "false" || v == "0" {
			c.File.Realtime = false
		}
		// 额外排除项只追加：硬编码的排除表（mtab、ld.so.cache 之类）不能被界面上
		// 一次误操作删掉，否则那些文件每轮都会产生一条无意义的变更。
		if v, _ := settingSvc.Get("change_exclude_globs"); strings.TrimSpace(v) != "" {
			c.File.IgnoreGlobs = append(c.File.IgnoreGlobs, splitCSV(v)...)
		}
		return c
	}
	ccfg := buildChangeConfig()
	changeStore, csErr := change.NewStore(db, ccfg.Quota.DiffTotalSize)
	snapshots := change.NewSnapshots(ccfg.Snapshot.Dir, ccfg.Snapshot.MaxPerDay, ccfg.Snapshot.MaxTotalSize)
	var correlator *change.Correlator
	var changeWatcher *change.Watcher
	var changeOnEvent func(ev change.Event)
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
		// 扇出回调独立成变量：重建 watcher 时必须复用同一份逻辑，否则重载一次
		// 之后变更就不再进信号总线，界面上看起来「采集正常但再也发现不了问题」。
		changeOnEvent = func(ev change.Event) {
			sig := changeSignal(ev)
			if rev, err := changeReviewer.Review(ev); err == nil && rev.Verdict == model.ChangeVerdictSuspicious {
				sig.Severity = model.SeverityHigh
				log.Printf("change: 可疑变更 %s（%.2f）→ 唤醒 Agent 分析", ev.Object.Key, rev.Score)
			}
			mgr.Bus.Publish(sig)
		}
		changeWatcher, csErr = change.NewWatcher(ccfg, changeStore, snapshots, changeOnEvent)
		if csErr != nil {
			log.Printf("sentinel: change watcher init failed: %v", csErr)
			changeWatcher = nil
		}
	}

	runner := &incidentRunner{
		agent:      ai.agent,
		store:      ai.store,
		db:         db,
		safety:     sm,
		dataDir:    dataDir,
		settingSvc: settingSvc,
		correlate:  correlator,
	}
	eng.Runner = runner
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
	// 事件挂起后的人工审批：续跑一个停在 confirm 级动作上的运行。
	// 用函数字段注入，api 包不需要知道 agent 引擎的存在。
	handler.Resume = runner.ResumeIncident
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

	// 采集设置保存后重建 watcher：各源在 Start 时按开关起 goroutine，运行中
	// 无法单独关掉，只能整体换一份。旧实例先 Stop（它会把在途事件写完），
	// 再按最新设置起新的。
	if changeAPI != nil && changeStore != nil {
		changeAPI.Reload = func() error {
			if changeWatcher != nil {
				changeWatcher.Stop()
			}
			w, err := change.NewWatcher(buildChangeConfig(), changeStore, snapshots, changeOnEvent)
			if err != nil {
				return err
			}
			w.Start(context.Background())
			changeWatcher = w
			bundle.changes = w
			changeAPI.Watcher = w
			return nil
		}
	}
	return bundle
}

// applyChangeSources applies the per-source switches stored by the change
// settings tab.
//
// Missing keys keep the default (enabled): a database written by an older build
// has no such setting, and "absent" must not silently mean "collect nothing".
func applyChangeSources(c *change.Config, settingSvc *service.SettingService) {
	raw, err := settingSvc.Get("change_sources")
	if err != nil || strings.TrimSpace(raw) == "" {
		return
	}
	var on map[string]bool
	if err := json.Unmarshal([]byte(raw), &on); err != nil {
		log.Printf("change: invalid change_sources setting: %v", err)
		return
	}
	setIf := func(key string, dst *bool) {
		if v, ok := on[key]; ok {
			*dst = v
		}
	}
	setIf("file", &c.File.Enabled)
	setIf("package", &c.Package.Enabled)
	setIf("service", &c.Service.Enabled)
	setIf("account", &c.Account.Enabled)
	setIf("firewall", &c.Firewall.Enabled)
	setIf("cron", &c.Cron.Enabled)
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
	// agent 指针存在不代表能用：模型可能压根没配（首装默认状态）。
	// 只看 r.agent == nil 会漏掉这种情况——那时 Agent.Run 会在更深处报错，
	// 引擎照常规判成 failed，用户看到的就是一条红色失败。
	if r.agent == nil || !r.agent.Ready() {
		out.NoModel = true
		return out, fmt.Errorf("AI 模型未配置，事件只记录不分析")
	}
	// 每日额度用尽时连启动都不做：让模型跑起来再掐掉，等于先花钱再告诉用户不能花。
	if limit := r.dailyTokenLimit(); limit > 0 {
		if _, used := r.dailyTokenUsage(); used >= limit {
			return trigger.RunResult{
				Summary: fmt.Sprintf("当日 token 额度（%d）已用尽，本次未启动 Agent。可在「设置 → 自主 Agent」调整额度，或等次日自动重置。",
					limit),
				BudgetExceeded: true,
			}, nil
		}
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
		UserID:     aiagent.SystemUserID,
		Username:   "sentinel",
		RoleName:   model.RoleAdmin,
		Unattended: true, // no human is watching at trigger time
		// assist 模式：碰到需人工确认的动作就挂起，等事件页上的人批准后续跑，
		// 而不是像定时任务那样直接拒绝。auto 模式保持拒绝（白名单之外的动作
		// 本来就不该由它自己决定）；shadow 只读，走不到这条路径。
		SuspendOnConfirm: req.Mode == model.ModeAssist,
		Title:            title,
		MaxSteps:         req.MaxSteps,
		Mode:             req.Mode,
		IncidentID:       req.Incident.ID,
		AllowedTools:     req.AllowedTools,
		DeniedTools:      req.DeniedTools,
		MaxWrites:        req.MaxWrites,
		AutoTools:        autoToolsFor(req.Mode, req.Policy.AutoTools),
		Evidence:         evidence,
		OnAction: func(ev aiagent.ActionEvent) {
			r.logAction(req.Incident.ID, "", ev)
			if ev.Verdict == aiagent.ActionAllow && ev.Risk >= aiagent.RiskLowWrite {
				out.Writes++
				r.journalWrite(req.Incident.ID, ev)
			}
		},
	}

	// token 预算：单事件上限来自策略，每日上限来自设置（0 = 不限）。
	// 超限第一时间停下——继续跑等于在用户不知情的情况下烧钱。
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 200000
	}
	dailyLimit := r.dailyTokenLimit()
	_, dailyUsed := r.dailyTokenUsage()
	var spentLast int
	opt.OnUsage = func(total int) error {
		if delta := total - spentLast; delta > 0 {
			r.safety.AddTokens(req.Incident.ID, delta)
			spentLast = total
		}
		if maxTokens > 0 && total > maxTokens {
			return fmt.Errorf("单事件 token 上限 %d 已超出（本次约 %d）", maxTokens, total)
		}
		if dailyLimit > 0 && dailyUsed+total > dailyLimit {
			return fmt.Errorf("当日 token 额度 %d 已用尽（已用 %d）", dailyLimit, dailyUsed+total)
		}
		return nil
	}

	res, err := r.agent.Send(ctx, opt, req.Prompt)
	if res != nil {
		out.SessionID = res.SessionID
		out.Steps = res.Steps
		out.Tools = res.Tools
		out.Skipped = res.Skipped
		out.Summary = res.Content
		out.Tokens = res.Tokens
		out.BudgetExceeded = res.BudgetExceeded
		// 挂起：本次运行停在需确认的动作上，等事件页上的人批准。
		// 报告不在这里归档——Resume 续跑完才是完整的处置记录。
		out.Suspended = res.Suspended
		out.ApprovalID = res.ApprovalID
		r.addDailyTokens(res.Tokens)
	}
	out.Writes = r.safety.WritesUsed(req.Incident.ID)

	// Reload the action log to fill verdicts that were written without a session
	// id (the session is only known after Send returns).
	r.db.Model(&model.ActionLog{}).Where("incident_id = ? AND session_id = ''", req.Incident.ID).
		Update("session_id", out.SessionID)

	// Backfill the decision chain: which signals this run was working from, and
	// what it believed. "What was done" without "on what basis" cannot be reviewed
	// after the fact, which is the only time anyone reads this table.
	if !out.Suspended {
		ids := make([]string, 0, len(req.Signals))
		for _, s := range req.Signals {
			ids = append(ids, s.ID)
		}
		if refs, jerr := json.Marshal(ids); jerr == nil {
			r.db.Model(&model.ActionLog{}).
				Where("incident_id = ? AND evidence_refs = ''", req.Incident.ID).
				Updates(map[string]any{
					"evidence_refs": string(refs),
					"hypothesis":    truncateStr(out.Summary, 300),
				})
		}
	}

	// Archive the report next to the scheduled-task reports. A suspended run has
	// not decided anything yet — the report is written after Resume finishes,
	// otherwise the archived file would say "waiting for approval" forever.
	if out.SessionID != "" && !out.Suspended {
		meta := aiagent.ReportMeta{
			TaskName:   req.Incident.Title,
			Prompt:     req.Prompt,
			Steps:      out.Steps,
			Tools:      out.Tools,
			Skipped:    out.Skipped,
			Duration:   time.Since(start),
			Unattended: true,
			Mode:       req.Incident.Mode,
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

// dailyTokenUsage reads today's consumed token count. Stored per day in the panel
// settings table so a restart does not hand out a fresh quota: the point of the
// cap is to bound spend, and an in-memory counter would reset exactly when a crash
// loop is burning tokens.
func (r *incidentRunner) dailyTokenUsage() (string, int) {
	date := time.Now().Format("2006-01-02")
	if r.settingSvc == nil {
		return date, 0
	}
	raw, _ := r.settingSvc.Get("agent_tokens_daily:" + date)
	n, _ := strconv.Atoi(strings.TrimSpace(raw))
	return date, n
}

// addDailyTokens adds to today's counter. Called once per run, not per turn: the
// per-turn cap is enforced in memory, and writing settings on every model turn
// would put a row update behind each step.
func (r *incidentRunner) addDailyTokens(n int) {
	if r.settingSvc == nil || n <= 0 {
		return
	}
	date, used := r.dailyTokenUsage()
	_ = r.settingSvc.Set(map[string]string{
		"agent_tokens_daily:" + date: strconv.Itoa(used + n),
	})
}

// dailyTokenLimit reads the daily cap (0 = unlimited).
func (r *incidentRunner) dailyTokenLimit() int {
	if r.settingSvc == nil {
		return 0
	}
	raw, _ := r.settingSvc.Get("agent_tokens_daily_limit")
	n, _ := strconv.Atoi(strings.TrimSpace(raw))
	return n
}

// resumeOptions rebuilds the run options for a suspended incident.
//
// Only the fields that constrain what the agent may do are restored — the tool
// allowlist, the write budget, the step budget and the approval policy. The
// prompt and evidence are deliberately left out: they are already in the session
// transcript, and re-injecting them would make the model re-read its own homework.
// Getting AllowedTools wrong here would be a security regression, so it comes from
// the same policy row the original run used, not from the incident.
func (r *incidentRunner) resumeOptions(inc model.Incident, p model.TriggerPolicy) aiagent.RunOptions {
	return aiagent.RunOptions{
		UserID:     aiagent.SystemUserID,
		Username:   "sentinel",
		RoleName:   model.RoleAdmin,
		Unattended: true,
		// 续跑时仍然可以在下一个需确认的动作上再停一次：一个策略里可能有
		// 多个待批动作，不能因为批准了一次就无限自我授权。
		SuspendOnConfirm: inc.Mode == model.ModeAssist,
		Title:            inc.Title,
		Mode:             inc.Mode,
		IncidentID:       inc.ID,
		AllowedTools:     splitCSV(p.AllowTools),
		DeniedTools:      splitCSV(p.DenyTools),
		MaxWrites:        p.MaxWrites,
		MaxSteps:         p.MaxSteps,
		AutoTools:        autoToolsFor(inc.Mode, p.AutoTools),
		OnAction: func(ev aiagent.ActionEvent) {
			// Session id is filled in by the caller after Resume returns (see
			// RunIncident); ActionEvent does not carry it.
			r.logAction(inc.ID, "", ev)
			if ev.Verdict == aiagent.ActionAllow && ev.Risk >= aiagent.RiskLowWrite {
				r.journalWrite(inc.ID, ev)
			}
		},
	}
}

// ResumeIncident continues a run that stopped for approval and closes the incident.
// Returns the final summary so the API can hand it straight back to the page.
func (r *incidentRunner) ResumeIncident(ctx context.Context, inc model.Incident, p model.TriggerPolicy,
	approvalID string, approved bool) (string, error) {
	if r.agent == nil {
		return "", fmt.Errorf("AI 模型未配置，无法继续执行")
	}
	start := time.Now()
	res, err := r.agent.Resume(ctx, r.resumeOptions(inc, p), approvalID, approved)
	if err != nil {
		// Leaving it in awaiting_approval after a failed resume would be worse than
		// failing: the page would keep showing a decision button that no longer works.
		r.db.Model(&model.Incident{}).Where("id = ?", inc.ID).Updates(map[string]any{
			"status":      model.IncidentFailed,
			"approval_id": "",
			"summary":     "续跑失败：" + err.Error(),
			"updated_at":  time.Now(),
		})
		return "", err
	}

	summary := strings.TrimSpace(res.Content)
	// A second confirm-level action in the same run: stay suspended instead of
	// closing the incident.
	if approved && res.Suspended {
		r.db.Model(&model.Incident{}).Where("id = ?", inc.ID).Updates(map[string]any{
			"status":      model.IncidentAwaitingApproval,
			"approval_id": res.ApprovalID,
			"summary":     summary,
			"updated_at":  time.Now(),
		})
		return summary, nil
	}

	reportRel := ""
	if res.SessionID != "" {
		meta := aiagent.ReportMeta{
			TaskName:   inc.Title,
			Prompt:     "（人工确认后续跑）",
			Steps:      res.Steps,
			Tools:      res.Tools,
			Skipped:    res.Skipped,
			Duration:   time.Since(start),
			Unattended: true,
			Mode:       inc.Mode,
			Status:     "success",
		}
		if md, berr := r.store.BuildReport(res.SessionID, meta); berr == nil {
			if art, serr := aiagent.ArchiveReport(r.dataDir(), "自主处置/"+sanitize(inc.Title), res.SessionID, md, meta); serr == nil {
				reportRel = art.RelHTML
			}
		}
	}

	status := model.IncidentResolved
	if !approved {
		status = model.IncidentSkipped
		if summary == "" {
			summary = "人工拒绝了待确认的操作"
		}
	}
	// 人工批准后执行的动作同样要复核：批准只代表"允许做"，不代表"做成了"。
	// 续跑路径以前直接置 resolved，等于把工具的自述当成了结果。
	verifyState := ""
	if approved && r.safety != nil {
		if ok, checks := r.safety.VerifyActions(ctx, inc.ID); !ok {
			status = model.IncidentUnverified
			verifyState = "action-unverified"
			if note := safety.SummarizeChecks(checks); note != "" {
				summary = strings.TrimSpace(summary) + "\n\n动作执行后复核未通过：\n" + note
			}
		}
	}
	now := time.Now()
	upd := map[string]any{
		"status":       status,
		"approval_id":  "",
		"summary":      summary,
		"session_id":   res.SessionID,
		"verify_state": verifyState,
		"updated_at":   now,
		"closed_at":    now,
	}
	if reportRel != "" {
		upd["report_rel"] = reportRel
	}
	r.db.Model(&model.Incident{}).Where("id = ?", inc.ID).Updates(upd)
	return summary, nil
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

	// 挂起等审批的事件必须收口。审批记录过期后 Approver 会拒绝续跑，但事件本身
	// 若还停在 awaiting_approval，页面就会一直显示"等待确认"而按钮点不动——
	// 这是最容易让人误以为"系统卡了"的状态。
	go func() {
		t := time.NewTicker(2 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				b.expireSuspendedIncidents()
			}
		}
	}()
}

// expireSuspendedIncidents closes incidents whose pending approval is gone or past
// its deadline, so the status never lies about whether anyone can act on it.
func (b *sentinelBundle) expireSuspendedIncidents() {
	if b.handler == nil || b.handler.DB == nil {
		return
	}
	var list []model.Incident
	b.handler.DB.Where("status = ?", model.IncidentAwaitingApproval).Limit(200).Find(&list)
	for _, inc := range list {
		reason := ""
		if inc.ApprovalID == "" {
			reason = "审批记录缺失"
		} else {
			var ap model.AIPendingApproval
			if err := b.handler.DB.First(&ap, "id = ?", inc.ApprovalID).Error; err != nil {
				reason = "审批记录不存在"
			} else if ap.Status != model.ApprovalPending {
				// 审批被 Sweep 标记过期时状态已经不是 pending，如果这里不区分，
				// 等待超时的用户只会看到「审批已结束但事件未收口」，等于没说
				// 到底是自己没处理，还是系统把记录弄丢了。
				if ap.Status == model.ApprovalExpired {
					reason = fmt.Sprintf("审批超时（%s 前未处理）", ap.ExpiresAt.Format("01-02 15:04"))
				} else {
					reason = "审批已结束但事件未收口"
				}
			} else if time.Now().After(ap.ExpiresAt) {
				reason = fmt.Sprintf("审批超时（%s 前未处理）", ap.ExpiresAt.Format("01-02 15:04"))
			}
		}
		if reason == "" {
			continue
		}
		now := time.Now()
		b.handler.DB.Model(&model.Incident{}).Where("id = ?", inc.ID).Updates(map[string]any{
			"status":      model.IncidentSkipped,
			"approval_id": "",
			"summary":     strings.TrimSpace(inc.Summary + "\n\n未执行：" + reason),
			"closed_at":   now,
			"updated_at":  now,
		})
		log.Printf("sentinel: incident %s closed as skipped (%s)", inc.ID, reason)
	}
}
