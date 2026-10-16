package service

import (
	"context"
	"errors"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"github.com/opsmini/opsmini/internal/model"
	"github.com/opsmini/opsmini/internal/repository"
)

// CronService handles scheduled task CRUD and scheduling.
//
// It schedules two kinds of jobs: "panel tasks" (kind=panel) which run a shell
// command in-process, and "agent tasks" (kind=agent) which hand a prompt to the
// AI Agent and archive the resulting report. System/user-sourced tasks are
// managed by the system cron and shown read-only by the panel.
type CronService struct {
	repo *repository.CronJobRepo
	cron *cron.Cron

	mu      sync.Mutex
	entries map[uint]cron.EntryID // jobID -> entryID

	// agent runs AI tasks; nil until the router injects it (agent tasks then log
	// an error instead of running).
	agent AgentRunner
	// notifier delivers AI task reports to the configured channels.
	notifier *Notifier
}

// agentFailLimit is the number of consecutive failures after which an AI task is
// disabled automatically (avoids failing every single day forever).
const agentFailLimit = 3

// SetAgentRunner injects the AI Agent executor. Must be called before the first
// agent task fires.
func (s *CronService) SetAgentRunner(r AgentRunner) { s.agent = r }

// SetNotifier injects the notification dispatcher used for AI task reports.
func (s *CronService) SetNotifier(n *Notifier) { s.notifier = n }

// NewCronService creates a CronService: loads enabled tasks and starts the scheduler.
func NewCronService(repo *repository.CronJobRepo) *CronService {
	s := &CronService{
		repo:    repo,
		cron:    cron.New(),
		entries: make(map[uint]cron.EntryID),
	}
	s.reload()
	s.cron.Start()
	return s
}

// Stop stops the scheduler.
func (s *CronService) Stop() { s.cron.Stop() }

// reload loads all enabled tasks.
func (s *CronService) reload() {
	jobs, err := s.repo.ListEnabled()
	if err != nil {
		log.Printf("cron reload: %v", err)
		return
	}
	for _, j := range jobs {
		s.register(j)
	}
}

// register adds a task to the scheduler (ignores system/user sources and invalid expressions).
func (s *CronService) register(j model.CronJob) {
	if (j.Kind != model.CronKindPanel && j.Kind != model.CronKindAgent) || !j.Enabled {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.entries[j.ID]; exists {
		return
	}
	id, err := s.cron.AddFunc(j.Schedule, func() { s.execute(j) })
	if err != nil {
		log.Printf("cron add %q: %v", j.Name, err)
		return
	}
	s.entries[j.ID] = id
}

// unregister removes a task from the scheduler.
func (s *CronService) unregister(id uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if eid, ok := s.entries[id]; ok {
		s.cron.Remove(eid)
		delete(s.entries, id)
	}
}

// execute runs a task and updates its last run time.
func (s *CronService) execute(j model.CronJob) {
	if j.Kind == model.CronKindAgent {
		s.executeAgent(j)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	start := time.Now()
	out, err := exec.CommandContext(ctx, "sh", "-c", j.Command).CombinedOutput()
	_ = out
	if err != nil {
		log.Printf("cron %q (id=%d) failed after %s: %v", j.Name, j.ID, time.Since(start), err)
	} else {
		log.Printf("cron %q (id=%d) done in %s", j.Name, j.ID, time.Since(start))
	}

	now := time.Now()
	if cur, e := s.repo.FindByID(j.ID); e == nil {
		cur.LastRun = &now
		_ = s.repo.Update(cur)
	}
}

// CronReq is the input for creating/updating a task.
//
// Command is required for panel tasks and Prompt for agent tasks; validation is
// done in validate() rather than by the binding tags, because which field is
// mandatory depends on Kind.
type CronReq struct {
	Name     string `json:"name" binding:"required"`
	Kind     string `json:"kind"`
	User     string `json:"user"`
	Schedule string `json:"schedule" binding:"required"`
	Command  string `json:"command"`
	Enabled  *bool  `json:"enabled"`

	// --- AI Agent tasks (kind=agent) ---
	Prompt         string `json:"prompt"`
	Notify         string `json:"notify"`
	MaxSteps       int    `json:"max_steps"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}

// validate checks the request against the selected task kind.
func (r CronReq) validate() error {
	switch r.Kind {
	case model.CronKindAgent:
		if strings.TrimSpace(r.Prompt) == "" {
			return errors.New("Agent 任务必须填写提示词")
		}
	case model.CronKindSystem, model.CronKindUser:
		return errors.New("系统/用户 crontab 任务由系统管理，面板不支持创建")
	default:
		if strings.TrimSpace(r.Command) == "" {
			return errors.New("面板任务必须填写执行命令")
		}
	}
	if strings.TrimSpace(r.Schedule) == "" {
		return errors.New("必须填写 cron 表达式")
	}
	return nil
}

// applyAgentFields copies the AI task fields onto the model.
func (r CronReq) applyAgentFields(j *model.CronJob) {
	j.Prompt = r.Prompt
	j.Notify = r.Notify
	j.MaxSteps = r.MaxSteps
	j.TimeoutSeconds = r.TimeoutSeconds
}

// Create creates a task.
func (s *CronService) Create(req CronReq) (*model.CronJob, error) {
	req.Kind = orDefault(req.Kind, model.CronKindPanel)
	if err := req.validate(); err != nil {
		return nil, err
	}
	j := &model.CronJob{
		Name:     req.Name,
		Kind:     req.Kind,
		User:     req.User,
		Schedule: req.Schedule,
		Command:  req.Command,
		Enabled:  req.Enabled == nil || *req.Enabled,
	}
	req.applyAgentFields(j)
	if err := s.repo.Create(j); err != nil {
		return nil, err
	}
	if j.Enabled {
		s.register(*j)
	}
	return j, nil
}

// Update updates a task and syncs the scheduler.
func (s *CronService) Update(id uint, req CronReq) (*model.CronJob, error) {
	j, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if req.Kind == "" {
		req.Kind = j.Kind
	}
	if err := req.validate(); err != nil {
		return nil, err
	}
	j.Name = req.Name
	j.Kind = req.Kind
	j.User = req.User
	j.Schedule = req.Schedule
	j.Command = req.Command
	req.applyAgentFields(j)
	if req.Enabled != nil {
		j.Enabled = *req.Enabled
	}
	// A manual save clears the previous failure streak.
	j.FailCount = 0
	j.LastError = ""
	if err := s.repo.Update(j); err != nil {
		return nil, err
	}
	// sync scheduler: remove the old entry first, then re-register as needed
	s.unregister(id)
	if j.Enabled {
		s.register(*j)
	}
	return j, nil
}

// Delete deletes a task and removes it from the scheduler.
func (s *CronService) Delete(id uint) error {
	s.unregister(id)
	return s.repo.Delete(id)
}

// List returns all tasks (including system/user sources, merged by SystemCronJobs).
func (s *CronService) List() ([]model.CronJob, error) { return s.repo.List() }

// RunNow executes a task immediately, outside its schedule. The run happens in
// the background so the API answers right away; the caller polls the task list
// (or the AI session) for the outcome.
func (s *CronService) RunNow(id uint) (*model.CronJob, error) {
	j, err := s.repo.FindByID(id)
	if err != nil {
		return nil, err
	}
	if j.Kind == model.CronKindSystem || j.Kind == model.CronKindUser {
		return nil, errors.New("系统/用户 crontab 任务不支持从面板触发")
	}
	go s.execute(*j)
	return j, nil
}

// executeAgent runs an AI Agent task unattended: the agent investigates with its
// read-only tools, refuses anything that would need a human approval, archives
// the report and pushes the summary to the configured channels.
func (s *CronService) executeAgent(j model.CronJob) {
	if s.agent == nil {
		log.Printf("cron %q (id=%d) is an AI task but no agent runner is configured", j.Name, j.ID)
		s.recordAgentRun(j, model.CronStatusFailed, "AI 智能体未启用（请在面板中配置模型）", "", "", nil)
		return
	}
	timeout := agentDefaultTimeout
	if j.TimeoutSeconds > 0 {
		timeout = time.Duration(j.TimeoutSeconds) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()
	res, err := s.agent.RunUnattended(ctx, AgentTask{
		ID:       j.ID,
		Name:     j.Name,
		Prompt:   j.Prompt,
		MaxSteps: j.MaxSteps,
		Timeout:  timeout,
	})
	elapsed := time.Since(start)

	status := model.CronStatusSuccess
	errMsg := ""
	if err != nil {
		status = model.CronStatusFailed
		errMsg = err.Error()
		if len(res.Skipped) == 0 {
			res.Skipped = nil
		}
	}
	log.Printf("cron %q (id=%d) AI task %s in %s (steps=%d tools=%d skipped=%d)",
		j.Name, j.ID, status, elapsed.Round(time.Second), res.Steps, res.Tools, len(res.Skipped))

	s.recordAgentRun(j, status, errMsg, res.SessionID, res.ReportRel, res.Skipped)
	s.notifyAgentReport(j, status, res, elapsed)
}

// agentDefaultTimeout bounds an AI task when the task does not set its own.
const agentDefaultTimeout = 10 * time.Minute

// recordAgentRun persists the outcome and auto-disables a repeatedly failing task.
func (s *CronService) recordAgentRun(j model.CronJob, status, errMsg, sessionID, reportRel string, skipped []string) {
	cur, err := s.repo.FindByID(j.ID)
	if err != nil {
		return
	}
	now := time.Now()
	cur.LastRun = &now
	cur.LastStatus = status
	cur.LastError = truncate(errMsg, 500)
	cur.SkippedTools = truncate(strings.Join(skipped, ", "), 500)
	if sessionID != "" {
		cur.LastSession = sessionID
	}
	if reportRel != "" {
		cur.LastReport = reportRel
	}
	if status == model.CronStatusSuccess {
		cur.FailCount = 0
	} else {
		cur.FailCount++
		// Stop the bleeding: a task that keeps failing would otherwise alert every
		// single day forever.
		if cur.FailCount >= agentFailLimit && cur.Enabled {
			cur.Enabled = false
			s.unregister(cur.ID)
			log.Printf("cron %q (id=%d) auto-disabled after %d consecutive failures",
				cur.Name, cur.ID, cur.FailCount)
		}
	}
	_ = s.repo.Update(cur)
}

// notifyAgentReport pushes the report summary to the task's channels.
func (s *CronService) notifyAgentReport(j model.CronJob, status string, res AgentTaskResult, elapsed time.Duration) {
	if s.notifier == nil {
		return
	}
	channels := strings.TrimSpace(j.Notify)
	if channels == "" {
		// Default to the in-panel bell: an unattended task nobody hears about is
		// useless.
		channels = NotifyChannelPanel
	}
	ok := status == model.CronStatusSuccess
	head := "AI 任务「" + j.Name + "」"
	if ok {
		head += "执行完成"
	} else {
		head += "执行失败"
	}
	var b strings.Builder
	b.WriteString("耗时 ")
	b.WriteString(elapsed.Round(time.Second).String())
	b.WriteString("，步数 ")
	b.WriteString(strconv.Itoa(res.Steps))
	b.WriteString("，工具调用 ")
	b.WriteString(strconv.Itoa(res.Tools))
	if len(res.Skipped) > 0 {
		b.WriteString("\n\n因无人值守被拒绝的操作（需人工确认）：")
		b.WriteString(strings.Join(res.Skipped, "、"))
	}
	b.WriteString("\n\n")
	b.WriteString(truncate(res.Content, 1500))

	level := "info"
	if !ok {
		level = "critical"
	}
	s.notifier.Send(NotifyMessage{
		Channels:    channels,
		Level:       level,
		PanelTitle:  head,
		PanelBody:   b.String(),
		MailSubject: "[OpsMini] " + head,
		MailBody:    b.String(),
		WebhookText: head + "\n" + truncate(res.Content, 600),
	})
}
