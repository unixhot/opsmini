package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/opsmini/opsmini/internal/model"
)

// AuditFunc records an audit entry. It matches service.AuditService.Record, so
// the caller can pass that method directly without this package importing the
// service layer.
type AuditFunc func(userID uint, username, action, resource, detail, ip string)

// Config tunes the ReAct kernel.
type Config struct {
	// MaxSteps caps the number of reason/act iterations per user turn.
	MaxSteps int
	// MaxToolSummary caps how many characters of a tool summary are fed back to
	// the model (protects the context window).
	MaxToolSummary int
	// StepTimeout is the per-tool execution budget.
	StepTimeout time.Duration
	// Snapshot returns a compact live system snapshot injected into the system
	// prompt so the model can orient itself before calling any tool.
	Snapshot func() string
}

// withDefaults fills zero values.
func (c Config) withDefaults() Config {
	if c.MaxSteps <= 0 {
		c.MaxSteps = 12
	}
	if c.MaxToolSummary <= 0 {
		c.MaxToolSummary = 6000
	}
	if c.StepTimeout <= 0 {
		c.StepTimeout = 120 * time.Second
	}
	return c
}

// RunOptions carries the per-request identity and streaming sink.
type RunOptions struct {
	SessionID string
	UserID    uint
	Username  string
	RoleName  string
	Context   string // current page context reported by the frontend
	DeepThink bool
	Emitter   Emitter

	// Unattended marks a scheduled task running with nobody watching. Tools that
	// would need a human approval are then REFUSED instead of suspending the run,
	// because there is no one to click the confirmation dialog.
	Unattended bool
	// Title overrides the auto-derived session title (used by scheduled tasks so
	// the report shows up as "每日巡检 · 2026-10-16").
	Title string
	// MaxSteps overrides the configured per-turn step budget (0 = engine default).
	MaxSteps int
}

// Agent is the ReAct kernel: it reasons with the model, invokes tools, feeds the
// observations back, and enforces the safety rails along the way.
type Agent struct {
	llm      *LLMClient
	reg      *Registry
	guard    *Guard
	store    *Store
	approver *Approver
	audit    AuditFunc
	policy   PolicyResolver
	hasPerm  func(roleName, perm string) bool
	compress func(sessionID string)
	cfg      Config
}

// NewAgent creates an Agent.
func NewAgent(llm *LLMClient, reg *Registry, guard *Guard, store *Store, approver *Approver, cfg Config) *Agent {
	return &Agent{
		llm:      llm,
		reg:      reg,
		guard:    guard,
		store:    store,
		approver: approver,
		cfg:      cfg.withDefaults(),
	}
}

// SetAuditor installs the audit hook.
func (a *Agent) SetAuditor(f AuditFunc) { a.audit = f }

// SetPolicyResolver installs the per-tool policy override resolver.
func (a *Agent) SetPolicyResolver(f PolicyResolver) { a.policy = f }

// SetPermissionChecker installs the RBAC checker used to filter tools.
func (a *Agent) SetPermissionChecker(f func(roleName, perm string) bool) { a.hasPerm = f }

// SetCompressor installs the transcript compressor invoked after a run.
func (a *Agent) SetCompressor(f func(sessionID string)) { a.compress = f }

// Registry exposes the tool registry.
func (a *Agent) Registry() *Registry { return a.reg }

// MaxSteps returns the configured per-turn step budget.
func (a *Agent) MaxSteps() int { return a.cfg.MaxSteps }

// Guard exposes the safety guard.
func (a *Agent) Guard() *Guard { return a.guard }

// Approver exposes the approval store.
func (a *Agent) Approver() *Approver { return a.approver }

// Store exposes the conversation store.
func (a *Agent) Store() *Store { return a.store }

// LLM exposes the model client (used by the settings page connection test).
func (a *Agent) LLM() *LLMClient { return a.llm }

// SendResult reports what happened during one Run/Resume call.
type SendResult struct {
	SessionID string
	Steps     int
	Tools     int
	Suspended bool
	Content   string
	// Skipped lists the tools refused because they need human approval while the
	// run is unattended.
	Skipped []string
}

// Send appends a user message and runs the ReAct loop for it.
func (a *Agent) Send(ctx context.Context, opt RunOptions, userMessage string) (*SendResult, error) {
	emit := opt.Emitter
	if emit == nil {
		emit = nopEmitter()
	}
	if err := a.llm.Validate(); err != nil {
		return nil, err
	}
	title := userMessage
	if opt.Title != "" {
		title = opt.Title
	}
	sess, err := a.store.EnsureSession(opt.SessionID, opt.UserID, title)
	if err != nil {
		return nil, err
	}
	opt.SessionID = sess.ID
	emit.Emit(Event{Type: EventSession, SessionID: sess.ID})

	if err := a.store.Append(sess.ID, Message{Role: model.AIRoleUser, Content: userMessage}); err != nil {
		return nil, err
	}
	// Auto-title from the first user message.
	if sess.Title == "" || sess.Title == "新会话" {
		sess.Title = truncateText(strings.TrimSpace(userMessage), 40)
	}

	history, err := a.store.History(sess.ID, 0)
	if err != nil {
		return nil, err
	}
	return a.loop(ctx, opt, sess, history, 0)
}

// Resume continues a suspended run after the human made a decision.
func (a *Agent) Resume(ctx context.Context, opt RunOptions, approvalID string, approved bool) (*SendResult, error) {
	emit := opt.Emitter
	if emit == nil {
		emit = nopEmitter()
	}
	rec, err := a.approver.Load(approvalID, opt.UserID)
	if err != nil {
		return nil, err
	}
	sess, err := a.store.GetSession(rec.SessionID, opt.UserID)
	if err != nil {
		return nil, err
	}
	opt.SessionID = sess.ID
	// The decision has been taken: the record must never be replayed.
	if err := a.approver.Resolve(rec, approved); err != nil {
		return nil, err
	}

	history, err := a.store.History(sess.ID, 0)
	if err != nil {
		return nil, err
	}
	callIdx, calls, ok := findSuspended(history, rec.ToolCallID)
	if !ok {
		return nil, errors.New("未找到待确认的工具调用，会话状态可能已过期")
	}

	// Answer the decided call, then explicitly skip the remaining calls of the
	// same assistant turn so the transcript stays valid for the model.
	var appended []Message
	if approved {
		appended = append(appended, a.runTool(ctx, opt, calls[callIdx], rec.Preview, true)...)
	} else {
		a.auditTool(opt, rec.ToolName, "reject", rec.Preview)
		appended = append(appended, toolMessage(calls[callIdx].ID, rec.ToolName,
			"用户拒绝执行该操作，已取消。请不要再次尝试相同的调用，改为向用户说明影响并给出替代建议。"))
		emit.Emit(Event{
			Type: EventToolResult, SessionID: sess.ID, ID: calls[callIdx].ID, Name: rec.ToolName,
			OK: boolPtr(false), Summary: "用户已拒绝该操作",
		})
	}
	for i := callIdx + 1; i < len(calls); i++ {
		appended = append(appended, toolMessage(calls[i].ID, calls[i].Function.Name,
			"未执行：同一轮的其它操作因需要人工确认而被跳过。如需执行请在下一轮重新发起。"))
	}
	if err := a.store.Append(sess.ID, appended...); err != nil {
		return nil, err
	}

	history, err = a.store.History(sess.ID, 0)
	if err != nil {
		return nil, err
	}
	// The approved call counts as one tool invocation; a rejected one does not.
	alreadyRan := 0
	if approved {
		alreadyRan = 1
	}
	return a.loop(ctx, opt, sess, history, alreadyRan)
}

// loop runs the reason → act → observe cycle until the model answers without
// requesting more tools, a confirmation is required, or the step budget is spent.
// toolsDone seeds the tool counter with calls that were already executed before
// the loop started (the approved call when resuming a suspended run).
func (a *Agent) loop(ctx context.Context, opt RunOptions, sess *model.AISession, history []Message, toolsDone int) (*SendResult, error) {
	emit := opt.Emitter
	if emit == nil {
		emit = nopEmitter()
	}
	res := &SendResult{SessionID: sess.ID, Tools: toolsDone}
	tools := a.availableTools(opt.RoleName)
	msgs := make([]Message, 0, len(history)+2)
	msgs = append(msgs, Message{Role: model.AIRoleSystem, Content: a.systemPrompt(opt)})
	msgs = append(msgs, history...)

	maxSteps := a.cfg.MaxSteps
	if opt.MaxSteps > 0 {
		maxSteps = opt.MaxSteps
	}
	for step := 1; step <= maxSteps; step++ {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		emit.Emit(Event{Type: EventStep, SessionID: sess.ID, Step: step})

		var streamed strings.Builder
		msg, usage, err := a.llm.ChatStream(ctx, msgs, tools, func(delta string) {
			streamed.WriteString(delta)
			emit.Emit(Event{Type: EventThinking, SessionID: sess.ID, Step: step, Delta: delta})
		})
		if err != nil {
			emit.Emit(Event{Type: EventError, SessionID: sess.ID, Message: err.Error()})
			a.store.Touch(sess, res.Steps, res.Tools)
			return res, err
		}
		if usage != nil {
			emit.Emit(Event{Type: EventThinking, SessionID: sess.ID, Step: step, Usage: usage})
		}
		if msg == nil {
			msg = &Message{Role: model.AIRoleAssistant, Content: streamed.String()}
		}
		res.Steps++
		msgs = append(msgs, *msg)
		if err := a.store.Append(sess.ID, *msg); err != nil {
			return res, err
		}

		// No tool requested: this is the final answer.
		if len(msg.ToolCalls) == 0 {
			res.Content = strings.TrimSpace(msg.Content)
			emit.Emit(Event{Type: EventFinal, SessionID: sess.ID, Step: step, Content: res.Content})
			a.finish(sess, res)
			return res, nil
		}

		// Execute the requested calls in order.
		for _, call := range msg.ToolCalls {
			call := call
			tool, ok := a.reg.Get(call.Function.Name)
			if !ok || !containsTool(tools, call.Function.Name) {
				reason := fmt.Sprintf("工具 %s 不可用（未注册或当前账号无权限）", call.Function.Name)
				out := toolMessage(call.ID, call.Function.Name, reason)
				msgs = append(msgs, out)
				_ = a.store.Append(sess.ID, out)
				emit.Emit(Event{Type: EventToolResult, SessionID: sess.ID, ID: call.ID, Name: call.Function.Name, OK: boolPtr(false), Summary: reason})
				continue
			}

			args := call.Args()
			preview := a.preview(tool, args)

			// L3: hard-forbidden patterns — refuse before anything else.
			if reason, rule := a.guard.Scan(tool, args); reason != "" {
				emit.Emit(Event{Type: EventToolCall, SessionID: sess.ID, ID: call.ID, Name: tool.Name,
					Title: tool.Title, Args: args, Risk: tool.Risk.String(), RiskCode: int(tool.Risk), Preview: preview})
				emit.Emit(Event{Type: EventToolResult, SessionID: sess.ID, ID: call.ID, Name: tool.Name,
					OK: boolPtr(false), Summary: reason})
				out := toolMessage(call.ID, tool.Name, reason+"。请不要尝试绕过该限制，改为向用户说明风险。")
				msgs = append(msgs, out)
				_ = a.store.Append(sess.ID, out)
				a.auditTool(opt, tool.Name, "deny:"+rule, preview)
				continue
			}

			// Decide whether the call may run unattended.
			override := ""
			if a.policy != nil {
				override = a.policy(tool.Name)
			}
			switch PolicyFor(tool, override) {
			case PolicyDeny:
				reason := fmt.Sprintf("工具 %s 已被管理员禁用", tool.Name)
				out := toolMessage(call.ID, tool.Name, reason)
				msgs = append(msgs, out)
				_ = a.store.Append(sess.ID, out)
				emit.Emit(Event{Type: EventToolResult, SessionID: sess.ID, ID: call.ID, Name: tool.Name, OK: boolPtr(false), Summary: reason})
				continue
			case PolicyConfirm:
				// Unattended run (scheduled task): nobody can approve, so refuse
				// the call, tell the model why, and keep going with the rest.
				if opt.Unattended {
					reason := fmt.Sprintf("无人值守的定时任务：工具 %s 属于「需人工确认」的操作，已被安全策略拒绝执行。请改用只读工具获取信息，并在结论中说明该操作需要人工介入。", tool.Name)
					emit.Emit(Event{Type: EventToolCall, SessionID: sess.ID, ID: call.ID, Name: tool.Name,
						Title: tool.Title, Args: args, Risk: tool.Risk.String(), RiskCode: int(tool.Risk), Preview: preview})
					emit.Emit(Event{Type: EventToolResult, SessionID: sess.ID, ID: call.ID, Name: tool.Name,
						OK: boolPtr(false), Summary: reason})
					out := toolMessage(call.ID, tool.Name, reason)
					msgs = append(msgs, out)
					_ = a.store.Append(sess.ID, out)
					if !containsString(res.Skipped, tool.Name) {
						res.Skipped = append(res.Skipped, tool.Name)
					}
					a.auditTool(opt, tool.Name, "unattended-deny", preview)
					continue
				}
				emit.Emit(Event{Type: EventToolCall, SessionID: sess.ID, ID: call.ID, Name: tool.Name,
					Title: tool.Title, Args: args, Risk: tool.Risk.String(), RiskCode: int(tool.Risk), Preview: preview})
				rec, aerr := a.approver.Create(sess.ID, opt.UserID, tool, call, args, preview)
				if aerr != nil {
					return res, aerr
				}
				// The remaining calls of this turn stay unanswered until the user
				// decides; Resume appends their "skipped" results in order.
				res.Suspended = true
				a.store.Touch(sess, res.Steps, res.Tools)
				emit.Emit(Event{
					Type: EventConfirm, SessionID: sess.ID, ID: call.ID, Name: tool.Name,
					Title: tool.Title, Args: args, Risk: tool.Risk.String(), RiskCode: int(tool.Risk),
					Preview: preview, ApprovalID: rec.ID, ExpiresAt: rec.ExpiresAt.Unix(), Pending: true,
				})
				return res, nil
			}

			// PolicyAuto: run it.
			emit.Emit(Event{Type: EventToolCall, SessionID: sess.ID, ID: call.ID, Name: tool.Name,
				Title: tool.Title, Args: args, Risk: tool.Risk.String(), RiskCode: int(tool.Risk), Preview: preview})
			out := a.runTool(ctx, opt, call, preview, true)
			res.Tools++
			msgs = append(msgs, out...)
			_ = a.store.Append(sess.ID, out...)
		}
	}

	// Step budget exhausted: report what is known and stop touching the host.
	final := fmt.Sprintf("已达到最大执行步数（%d），为避免误操作已停止继续调用工具。以上是目前掌握的情况，如需继续请再次提问。", maxSteps)
	emit.Emit(Event{Type: EventFinal, SessionID: sess.ID, Content: final})
	res.Content = final
	a.finish(sess, res)
	return res, nil
}

// finish updates session bookkeeping and compresses the transcript when needed.
func (a *Agent) finish(sess *model.AISession, res *SendResult) {
	a.store.Touch(sess, res.Steps, res.Tools)
	if a.compress != nil && res.SessionID != "" {
		if a.store.Count(res.SessionID) > int64(a.store.keepVerbatim)+12 {
			a.compress(res.SessionID)
		}
	}
}

// runTool executes a tool and returns the resulting tool message.
func (a *Agent) runTool(ctx context.Context, opt RunOptions, call ToolCall, preview string, emitResult bool) []Message {
	tool, ok := a.reg.Get(call.Function.Name)
	if !ok {
		return []Message{toolMessage(call.ID, call.Function.Name, "工具不存在")}
	}
	exec := ExecContext{
		Context:   ctx,
		SessionID: opt.SessionID,
		UserID:    opt.UserID,
		Username:  opt.Username,
	}
	if opt.Emitter != nil {
		exec.Emit = func(e Event) {
			e.SessionID = opt.SessionID
			opt.Emitter.Emit(e)
		}
	}

	stepCtx, cancel := context.WithTimeout(ctx, a.cfg.StepTimeout)
	defer cancel()
	exec.Context = stepCtx

	start := time.Now()
	result, err := tool.Execute(exec, call.Args())
	elapsed := time.Since(start)

	if err != nil {
		summary := fmt.Sprintf("工具 %s 执行失败：%s", tool.Name, err.Error())
		if opt.Emitter != nil && emitResult {
			opt.Emitter.Emit(Event{Type: EventToolResult, SessionID: opt.SessionID, ID: call.ID,
				Name: tool.Name, OK: boolPtr(false), Summary: summary, Duration: elapsed.Milliseconds()})
		}
		a.auditTool(opt, tool.Name, "error", preview+"｜"+err.Error())
		return []Message{toolMessage(call.ID, tool.Name, summary)}
	}
	if len(result.Summary) > a.cfg.MaxToolSummary {
		result.Summary = truncateText(result.Summary, a.cfg.MaxToolSummary)
	}
	if opt.Emitter != nil && emitResult {
		opt.Emitter.Emit(Event{Type: EventToolResult, SessionID: opt.SessionID, ID: call.ID,
			Name: tool.Name, OK: boolPtr(result.OK), Summary: result.Summary, Detail: result.Detail,
			Duration: elapsed.Milliseconds()})
	}
	a.auditTool(opt, tool.Name, "execute", preview)
	return []Message{toolMessage(call.ID, tool.Name, result.Summary)}
}

// auditTool records a tool invocation in the audit log.
func (a *Agent) auditTool(opt RunOptions, toolName, action, detail string) {
	if a.audit == nil {
		return
	}
	a.audit(opt.UserID, opt.Username, "ai_tool", toolName, truncateText(action+": "+detail, 500), "")
}

// preview renders the human-readable description of a call.
func (a *Agent) preview(t *Tool, args map[string]any) string {
	if t.Preview == nil {
		raw, _ := json.Marshal(args)
		return t.Name + " " + string(raw)
	}
	if s := t.Preview(args); s != "" {
		return s
	}
	raw, _ := json.Marshal(args)
	return t.Name + " " + string(raw)
}

// availableTools returns the tools visible to a role: RBAC-filtered and with
// administrator-denied tools removed.
func (a *Agent) availableTools(roleName string) []Tool {
	all := a.reg.List()
	out := make([]Tool, 0, len(all))
	for _, t := range all {
		if t.Perm != "" && a.hasPerm != nil && !a.hasPerm(roleName, t.Perm) {
			continue
		}
		if a.policy != nil && a.policy(t.Name) == "deny" {
			continue
		}
		out = append(out, *t)
	}
	return out
}

// systemPrompt builds the agent's system prompt.
func (a *Agent) systemPrompt(opt RunOptions) string {
	var b strings.Builder
	b.WriteString(agentSystemPrompt)
	if opt.DeepThink {
		b.WriteString("\n\n" + deepThinkPrompt)
	}
	if opt.Context != "" {
		b.WriteString("\n\n【当前用户所在页面】" + opt.Context)
	}
	if a.cfg.Snapshot != nil {
		if snap := a.cfg.Snapshot(); snap != "" {
			b.WriteString("\n\n" + snap)
		}
	}
	return b.String()
}

// agentSystemPrompt is the base system prompt of the OpsMini Agent.
const agentSystemPrompt = `你是 OpsMini 运维面板内置的 AI 运维 Agent（智能体），具备在服务器上"观察"和"操作"的真实能力。

工作方式：
1. 你必须通过调用工具来获取真实数据，禁止凭经验猜测主机状态、进程、端口、磁盘或日志内容。
2. 遵循 ReAct 循环：先分析问题 → 调用工具获取事实 → 根据结果决定下一步，直到得出确定结论。
3. 尽量先用只读工具（readonly）定位根因，再考虑是否需要写操作。
4. 一次可调用多个工具；没有必要时不要重复调用同一个工具。
5. 高风险操作（如执行命令、删除文件、修改防火墙）系统会先让用户确认，你需要把"要做什么、为什么、影响是什么"讲清楚。
6. 工具返回失败或被拒绝时，不要反复重试，应说明原因并给出替代方案。

回答要求：
- 用简体中文，结构清晰，先给结论再给依据。
- 引用数据时给出具体数值和来源（哪个工具、哪个文件、哪个进程）。
- 给出可执行建议时使用代码块；不要编造命令输出。
- 如果信息不足，明确说明还缺什么，并主动调用工具去补齐。`

// deepThinkPrompt nudges the model to show its reasoning.
const deepThinkPrompt = `本次回答请展示你的推理过程：分步骤说明你的判断依据和每一步的目的，再给出最终结论。`

// toolMessage builds a tool-role message.
func toolMessage(callID, name, content string) Message {
	return Message{Role: model.AIRoleTool, ToolCallID: callID, Name: name, Content: content}
}

// containsTool reports whether the name is present in the tool slice.
func containsTool(tools []Tool, name string) bool {
	for _, t := range tools {
		if t.Name == name {
			return true
		}
	}
	return false
}

// findSuspended locates the assistant message holding the suspended call and
// derives how many calls of that turn were already answered: because results are
// appended in order and the run stops at the first unconfirmed call, the number
// of tool messages directly following the assistant message is the index of the
// suspended call.
func findSuspended(history []Message, toolCallID string) (callIdx int, calls []ToolCall, ok bool) {
	for i, m := range history {
		if m.Role != model.AIRoleAssistant || len(m.ToolCalls) == 0 {
			continue
		}
		for _, c := range m.ToolCalls {
			if c.ID != toolCallID {
				continue
			}
			answered := 0
			for k := i + 1; k < len(history); k++ {
				if history[k].Role != model.AIRoleTool {
					break
				}
				answered++
			}
			return answered, m.ToolCalls, true
		}
	}
	return 0, nil, false
}

// containsString reports whether the slice already holds the value.
func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// boolPtr returns a pointer to v (used for tri-state JSON fields).
func boolPtr(v bool) *bool { return &v }
