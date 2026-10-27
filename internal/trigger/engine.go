// Package trigger decides which incidents wake the agent, in which mode, with
// which tool subset and with which budget. Policies are data (SQLite rows), not
// code, so an administrator can dial autonomy up or down per scenario.
package trigger

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/safety"
)

// RunRequest is what the engine hands to the agent runtime.
type RunRequest struct {
	Incident     model.Incident
	Policy       model.TriggerPolicy
	Mode         string
	Prompt       string
	AllowedTools []string
	DeniedTools  []string
	MaxWrites    int
	AutoTools    []string
	MaxSteps     int
	MaxSeconds   int
	MaxTokens    int
	Signals      []model.Signal
	Verify       []safety.Assertion
}

// RunResult is the outcome of one triggered run.
type RunResult struct {
	SessionID    string
	Summary      string
	ReportRel    string
	Steps        int
	Tools        int
	Writes       int
	Skipped      []string
	VerifyOK     bool
	VerifyDetail []string
	Error        string
}

// Runner is implemented by the AI agent layer (injected by the router to keep
// this package free of a dependency on the agent engine).
type Runner interface {
	RunIncident(ctx context.Context, req RunRequest) (RunResult, error)
}

// Engine matches incidents against policies and runs them one at a time.
type Engine struct {
	DB      *gorm.DB
	Safety  *safety.Manager
	Runner  Runner
	Notify  func(inc model.Incident, text string)
	Learning bool

	mu    sync.Mutex
	sem   chan struct{}
	queue chan queued
	once  sync.Once
	last  map[string]time.Time
}

// queued is one incident waiting for its turn. Runs are serialized on a single
// host, so a burst of incidents must queue instead of being dropped.
type queued struct {
	inc    model.Incident
	policy model.TriggerPolicy
}

// NewEngine creates the trigger engine. Runs are serialized: on a single host
// two agents must never edit the system at the same time.
func NewEngine(db *gorm.DB, sm *safety.Manager) *Engine {
	return &Engine{
		DB: db, Safety: sm,
		sem:   make(chan struct{}, 1),
		queue: make(chan queued, 32),
		last:  map[string]time.Time{},
	}
}

// --- policy storage ----------------------------------------------------------

// SeedDefaults inserts the shipped policies when the table is empty.
func (e *Engine) SeedDefaults() error {
	if e.DB == nil {
		return nil
	}
	var n int64
	e.DB.Model(&model.TriggerPolicy{}).Count(&n)
	if n > 0 {
		return nil
	}
	for _, p := range defaultPolicies() {
		if err := e.DB.Create(&p).Error; err != nil {
			return err
		}
	}
	return nil
}

// List returns all policies.
func (e *Engine) List() ([]model.TriggerPolicy, error) {
	var out []model.TriggerPolicy
	err := e.DB.Order("id").Find(&out).Error
	return out, err
}

// Update changes the mutable fields of a policy.
func (e *Engine) Update(id uint, in model.TriggerPolicy) (*model.TriggerPolicy, error) {
	var p model.TriggerPolicy
	if err := e.DB.First(&p, id).Error; err != nil {
		return nil, err
	}
	if in.Mode != "" {
		p.Mode = in.Mode
	}
	if in.MinSeverity != "" {
		p.MinSeverity = in.MinSeverity
	}
	if in.Family != "" {
		p.Family = in.Family
	}
	if in.Description != "" {
		p.Description = in.Description
	}
	if in.Prompt != "" {
		p.Prompt = in.Prompt
	}
	if in.Verify != "" {
		p.Verify = in.Verify
	}
	if in.AllowTools != "" {
		p.AllowTools = in.AllowTools
	}
	if in.DenyTools != "" {
		p.DenyTools = in.DenyTools
	}
	if in.AutoTools != "" {
		p.AutoTools = in.AutoTools
	}
	if in.Notify != "" {
		p.Notify = in.Notify
	}
	p.Enabled = in.Enabled
	if in.CooldownSec > 0 {
		p.CooldownSec = in.CooldownSec
	}
	if in.MaxWrites > 0 {
		p.MaxWrites = in.MaxWrites
	}
	if in.MaxSteps > 0 {
		p.MaxSteps = in.MaxSteps
	}
	if in.MaxSeconds > 0 {
		p.MaxSeconds = in.MaxSeconds
	}
	if in.MaxTokens > 0 {
		p.MaxTokens = in.MaxTokens
	}
	if err := ValidatePolicy(&p); err != nil {
		return nil, err
	}
	p.UpdatedAt = time.Now()
	if err := e.DB.Save(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// Create adds a new policy from the UI.
func (e *Engine) Create(in model.TriggerPolicy) (*model.TriggerPolicy, error) {
	p := in
	if p.CooldownSec == 0 {
		p.CooldownSec = 1800
	}
	if p.MaxWrites == 0 {
		p.MaxWrites = 1
	}
	if p.MaxSteps == 0 {
		p.MaxSteps = 12
	}
	if p.MaxSeconds == 0 {
		p.MaxSeconds = 300
	}
	if p.MaxTokens == 0 {
		p.MaxTokens = 150000
	}
	if p.MinSeverity == "" {
		p.MinSeverity = model.SeverityHigh
	}
	if p.Mode == "" {
		p.Mode = model.ModeShadow
	}
	if err := ValidatePolicy(&p); err != nil {
		return nil, err
	}
	var n int64
	e.DB.Model(&model.TriggerPolicy{}).Where("name = ?", p.Name).Count(&n)
	if n > 0 {
		return nil, fmt.Errorf("策略名 %q 已存在", p.Name)
	}
	if err := e.DB.Create(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// Delete removes a policy. Incidents already produced by it keep their policy
// name for the record.
func (e *Engine) Delete(id uint) error {
	var p model.TriggerPolicy
	if err := e.DB.First(&p, id).Error; err != nil {
		return err
	}
	return e.DB.Delete(&model.TriggerPolicy{}, id).Error
}

// knownFamilies are the scenario keys collectors actually emit. A policy on any
// other family would never match, so the UI only offers these.
var knownFamilies = []string{
	"webshell", "auth.anomaly", "proc.anomaly", "net.anomaly", "log_growth", "metric",
}

// KnownFamilies returns the scenario keys a policy can target.
func KnownFamilies() []string { return append([]string(nil), knownFamilies...) }

// ValidatePolicy rejects configurations that would silently do nothing or that
// would hand the agent more power than the mode allows.
func ValidatePolicy(p *model.TriggerPolicy) error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("策略名不能为空")
	}
	if len(p.Name) > 96 {
		return fmt.Errorf("策略名过长（最多 96 字符）")
	}
	if strings.TrimSpace(p.Family) == "" {
		return fmt.Errorf("必须选择适用场景（family）")
	}
	ok := false
	for _, f := range knownFamilies {
		if f == p.Family {
			ok = true
			break
		}
	}
	if !ok {
		return fmt.Errorf("未知场景 %q，可选：%s", p.Family, strings.Join(knownFamilies, " / "))
	}
	switch p.Mode {
	case model.ModeShadow, model.ModeAssist, model.ModeAuto:
	default:
		return fmt.Errorf("非法的运行模式 %q（可选 shadow / assist / auto）", p.Mode)
	}
	switch p.MinSeverity {
	case model.SeverityInfo, model.SeverityLow, model.SeverityMedium, model.SeverityHigh, model.SeverityCritical:
	default:
		return fmt.Errorf("非法的等级 %q", p.MinSeverity)
	}
	if p.CooldownSec < 0 || p.CooldownSec > 86400 {
		return fmt.Errorf("冷却时间需在 0~86400 秒之间")
	}
	if p.MaxWrites < 0 || p.MaxWrites > 20 {
		return fmt.Errorf("写动作上限需在 0~20 之间")
	}
	if p.MaxSteps < 1 || p.MaxSteps > 50 {
		return fmt.Errorf("最大步数需在 1~50 之间")
	}
	if p.MaxSeconds < 30 || p.MaxSeconds > 3600 {
		return fmt.Errorf("最长运行时间需在 30~3600 秒之间")
	}
	for _, f := range []struct {
		name string
		val  string
	}{{"allow_tools", p.AllowTools}, {"deny_tools", p.DenyTools},
		{"auto_tools", p.AutoTools}, {"verify", p.Verify}} {
		if strings.TrimSpace(f.val) == "" {
			continue
		}
		var arr []json.RawMessage
		if err := json.Unmarshal([]byte(f.val), &arr); err != nil {
			return fmt.Errorf("%s 必须是 JSON 数组，例如 [\"quarantine_file\"]", f.name)
		}
	}
	// A verify block that cannot be parsed would silently disable verification.
	if strings.TrimSpace(p.Verify) != "" && !strings.HasPrefix(strings.TrimSpace(p.Verify), "[") {
		return fmt.Errorf("verify 必须是 JSON 数组")
	}
	return nil
}

// --- dispatch ----------------------------------------------------------------

// Match returns the first enabled policy that covers the incident severity.
func (e *Engine) Match(inc model.Incident) *model.TriggerPolicy {
	var list []model.TriggerPolicy
	e.DB.Where("enabled = ? AND family = ?", true, inc.Family).Find(&list)
	for i := range list {
		p := &list[i]
		if model.SeverityRank(inc.Severity) >= model.SeverityRank(p.MinSeverity) {
			return p
		}
	}
	return nil
}

// OnIncident is called by the aggregator for every signal fold.
//
// It decides whether the incident deserves an agent run, but does not run it
// here: runs are serialized, so work is queued and picked up by one worker.
func (e *Engine) OnIncident(inc model.Incident, sig model.Signal) {
	if e.Runner == nil || e.DB == nil {
		return
	}
	if e.Learning {
		e.DB.Model(&model.Incident{}).Where("id = ?", inc.ID).
			Update("summary", "学习期：仅记录信号，不触发 Agent")
		return
	}
	p := e.Match(inc)
	if p == nil {
		return
	}

	e.mu.Lock()
	last := e.last[p.Name]
	if p.CooldownSec > 0 && time.Since(last) < time.Duration(p.CooldownSec)*time.Second {
		e.mu.Unlock()
		return
	}
	e.last[p.Name] = time.Now()
	e.mu.Unlock()

	// Queue instead of dropping: a burst of incidents must all be handled, one
	// after another (a single host cannot be edited by two agents at once).
	e.once.Do(func() { go e.worker() })
	select {
	case e.queue <- queued{inc: inc, policy: *p}:
	default:
		log.Printf("trigger: queue full, incident %s dropped", inc.ID)
	}
}

// worker consumes queued incidents one at a time.
func (e *Engine) worker() {
	for q := range e.queue {
		// The incident may have been resolved, ignored or taken over while it was
		// waiting: re-read the authoritative row before spending tokens.
		var cur model.Incident
		if err := e.DB.First(&cur, "id = ?", q.inc.ID).Error; err != nil {
			continue
		}
		switch cur.Status {
		case model.IncidentIgnored, model.IncidentTakenOver, model.IncidentResolved, model.IncidentFailed:
			continue
		}
		e.sem <- struct{}{}
		e.run(cur, q.policy)
		<-e.sem
	}
}

// run executes one incident end to end: prepare the run options, call the agent,
// verify the outcome, then update the incident.
func (e *Engine) run(inc model.Incident, p model.TriggerPolicy) {
	var signals []model.Signal
	e.DB.Where("incident_id = ?", inc.ID).Order("observed_at").Limit(50).Find(&signals)

	mode := p.Mode
	if e.Safety != nil && e.Safety.Paused() && mode != model.ModeShadow {
		// Kill switch: keep investigating, never write.
		mode = model.ModeShadow
	}

	var asserts []safety.Assertion
	if strings.TrimSpace(p.Verify) != "" {
		if err := json.Unmarshal([]byte(p.Verify), &asserts); err != nil {
			log.Printf("trigger: policy %s has invalid verify spec: %v", p.Name, err)
		}
	}

	req := RunRequest{
		Incident:     inc,
		Policy:       p,
		Mode:         mode,
		Prompt:       strings.ReplaceAll(p.Prompt, "{title}", inc.Title),
		AllowedTools: splitJSON(p.AllowTools),
		DeniedTools:  splitJSON(p.DenyTools),
		MaxWrites:    p.MaxWrites,
		AutoTools:    splitJSON(p.AutoTools),
		MaxSteps:     p.MaxSteps,
		MaxSeconds:   p.MaxSeconds,
		MaxTokens:    p.MaxTokens,
		Signals:      signals,
		Verify:       asserts,
	}

	e.DB.Model(&model.Incident{}).Where("id = ?", inc.ID).Updates(map[string]any{
		"status":      model.IncidentTriggered,
		"policy_id":   p.ID,
		"policy_name": p.Name,
		"mode":        mode,
	})
	e.DB.Model(&model.TriggerPolicy{}).Where("id = ?", p.ID).Update("last_run_at", time.Now())

	runRow := model.AgentRun{
		IncidentID: inc.ID, PolicyName: p.Name, Mode: mode,
		Status: "running", StartedAt: time.Now(),
	}
	e.DB.Create(&runRow)

	ctx := context.Background()
	if req.MaxSeconds > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, time.Duration(req.MaxSeconds)*time.Second)
		defer cancel()
	}
	e.DB.Model(&model.Incident{}).Where("id = ?", inc.ID).
		Update("status", model.IncidentRunning)

	res, err := e.Runner.RunIncident(ctx, req)
	fin := time.Now()

	upd := map[string]any{
		"status":      "success",
		"session_id":  res.SessionID,
		"steps":       res.Steps,
		"tools":       res.Tools,
		"writes":      res.Writes,
		"summary":     res.Summary,
		"report_rel":  res.ReportRel,
		"finished_at": fin,
	}
	if len(res.Skipped) > 0 {
		if b, jerr := json.Marshal(res.Skipped); jerr == nil {
			upd["skipped"] = string(b)
		}
	}
	status := model.IncidentResolved
	if err != nil {
		upd["status"] = "failed"
		upd["error"] = err.Error()
		status = model.IncidentFailed
	}
	e.DB.Model(&model.AgentRun{}).Where("id = ?", runRow.ID).Updates(upd)

	// Quality gate: a run that produced no usable conclusion is a failure, not a
	// success with an empty report. This catches models that lose the thread.
	verifyState := ""
	if err == nil && len(strings.TrimSpace(res.Summary)) < 20 {
		status = model.IncidentFailed
		verifyState = "empty-report"
	}

	// Post-action verification: an action is not "done" until the assertions pass.
	if len(req.Verify) > 0 && err == nil && verifyState == "" {
		ok, detail := e.Safety.Verify(ctx, inc.ID, req.Verify)
		res.VerifyOK = ok
		res.VerifyDetail = detail
		verifyState = "passed"
		if !ok {
			verifyState = "failed"
			status = model.IncidentFailed
		}
		e.DB.Model(&model.AgentRun{}).Where("id = ?", runRow.ID).
			Update("summary", res.Summary+"\n\n验证："+strings.Join(detail, "；"))
	}

	incUpd := map[string]any{
		"status":       status,
		"session_id":   res.SessionID,
		"report_rel":   res.ReportRel,
		"summary":      res.Summary,
		"verify_state": verifyState,
		"updated_at":   time.Now(),
	}
	if status == model.IncidentResolved || status == model.IncidentFailed {
		incUpd["closed_at"] = fin
	}
	e.DB.Model(&model.Incident{}).Where("id = ?", inc.ID).Updates(incUpd)

	if e.Notify != nil {
		text := fmt.Sprintf("【%s】%s — %s", strings.ToUpper(inc.Severity), inc.Title, mode)
		if res.Summary != "" {
			text += "\n" + truncate(res.Summary, 400)
		}
		if verifyState == "failed" {
			text += "\n⚠ 验证未通过，需人工确认"
		}
		if err != nil {
			text += "\n执行失败：" + err.Error()
		}
		e.Notify(inc, text)
	}
}

// --- defaults ----------------------------------------------------------------

func defaultPolicies() []model.TriggerPolicy {
	readTools := `["sys_overview","list_processes","list_ports","list_disks","list_services","dir_usage","read_file","search_logs","list_network"]`
	return []model.TriggerPolicy{
		{
			Name: "webshell-respond", Enabled: true, Family: "webshell",
			MinSeverity: model.SeverityHigh, Mode: model.ModeAssist,
			CooldownSec: 3600, MaxWrites: 4, MaxSteps: 12, MaxSeconds: 300, MaxTokens: 150000,
			AllowTools: strings.TrimSuffix(readTools, "]") + `,"quarantine_file","freeze_process","block_ip","snapshot_path"]`,
			AutoTools:  `["quarantine_file","freeze_process","block_ip"]`,
			DenyTools:  `["exec_command","delete_path"]`,
			Verify:     `[{"type":"no_new_signal","family":"webshell","window":"30m"}]`,
			Notify:     "panel,email",
			Description: "疑似 WebShell：先取证，再隔离文件、冻结进程、临时封禁来源 IP（均可还原）",
			Prompt: "事件「{title}」已由采集器确认，请按 playbook 处置：\n" +
				"1) 先只读取证：文件内容与哈希、属主、派生进程链路、相关访问日志；\n" +
				"2) 判断是否为 webshell 或误报，说明依据；\n" +
				"3) 若是：quarantine_file 隔离文件（不要删除）、freeze_process 冻结可疑进程（不要 kill）、block_ip 临时封禁来源 IP；\n" +
				"4) 明确列出后续需要人工完成的深处置项；\n" +
				"5) 不要执行通用命令，不要修改任何配置。",
		},
		{
			Name: "auth-anomaly", Enabled: true, Family: "auth.anomaly",
			MinSeverity: model.SeverityHigh, Mode: model.ModeAssist,
			CooldownSec: 1800, MaxWrites: 2, MaxSteps: 10, MaxSeconds: 240, MaxTokens: 120000,
			AllowTools: strings.TrimSuffix(readTools, "]") + `,"block_ip"]`,
			AutoTools:  `["block_ip"]`,
			DenyTools:  `["exec_command","write_file","delete_path","modify_sshd"]`,
			Verify:     `[{"type":"no_new_signal","family":"auth.anomaly","window":"15m"}]`,
			Notify:     "panel,email",
			Description: "登录行为异常：取证登录来源与现场，必要时临时封禁 IP",
			Prompt: "事件「{title}」已确认，请按 playbook 处置：\n" +
				"1) 只读取证：谁在什么时候从哪里登录、认证方式、当前会话、最近命令（如可读）；\n" +
				"2) 判断是正常运维、误报还是入侵，说明依据；\n" +
				"3) 若高度可疑：block_ip 临时封禁来源 IP（带 TTL）；**不要**修改 sshd 配置、不要删除 authorized_keys；\n" +
				"4) 列出需人工确认的深处置项（如吊销 key、禁用密码登录）；\n" +
				"5) 不要执行通用命令。",
		},
		{
			Name: "proc-anomaly", Enabled: true, Family: "proc.anomaly",
			MinSeverity: model.SeverityCritical, Mode: model.ModeAssist,
			CooldownSec: 1800, MaxWrites: 3, MaxSteps: 10, MaxSeconds: 240, MaxTokens: 120000,
			AllowTools: strings.TrimSuffix(readTools, "]") + `,"freeze_process"]`,
			DenyTools:  `["exec_command","delete_path"]`,
			Verify:     `[{"type":"process_absent","pattern":"minerd"}]`,
			Notify:     "panel",
			Description: "可疑进程（挖矿/反弹 shell/web 用户派生 shell）：冻结而非杀",
			Prompt: "事件「{title}」已确认，请处置：\n1) 只读取证进程 cmdline、父子链、打开的文件、网络连接；\n" +
				"2) 判断是否恶意，说明依据；\n3) 若确认恶意：freeze_process 冻结（不要 kill）；\n" +
				"4) 列出需人工确认的清理与持久化排查项。",
		},
		{
			Name: "net-anomaly", Enabled: false, Family: "net.anomaly",
			MinSeverity: model.SeverityHigh, Mode: model.ModeShadow,
			CooldownSec: 1800, MaxWrites: 1, MaxSteps: 8, MaxSeconds: 180, MaxTokens: 80000,
			AllowTools: readTools, DenyTools: `["exec_command"]`,
			Notify:     "panel",
			Description: "异常外联（默认关闭）：仅观察",
			Prompt: "事件「{title}」：请只读排查该外联的发起进程、目标与是否为已知业务，不要做任何写操作。",
		},
		{
			Name: "log-growth", Enabled: true, Family: "log_growth",
			MinSeverity: model.SeverityHigh, Mode: model.ModeShadow,
			CooldownSec: 1800, MaxWrites: 1, MaxSteps: 8, MaxSeconds: 180, MaxTokens: 80000,
			AllowTools: strings.TrimSuffix(readTools, "]") + `,"truncate_log"]`,
			AutoTools:  `["truncate_log"]`,
			DenyTools:  `["exec_command","delete_path"]`,
			Verify:     `[{"type":"disk_usage_lt","path":"/var","value":85}]`,
			Notify:     "panel",
			Description: "日志暴涨（影子模式）：找出持有进程与原因，只记录打算怎么做",
			Prompt: "事件「{title}」：请只读排查是哪个进程在写、写了什么模板、是否日志轮转失效；" +
				"给出处置建议（轮转/截断/重启应用），影子模式下不要执行任何写操作。",
		},
	}
}

func splitJSON(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
