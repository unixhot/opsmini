package v1

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/opsmini/opsmini/internal/aiagent"
	"github.com/opsmini/opsmini/internal/pkg/response"
)

// Settings gives the agent endpoints access to the panel settings and secrets.
type Settings struct {
	Get    func(key string) string
	Set    func(map[string]string) error
	Ensure func(key string) (string, error)
}

// AgentDeps groups the AgentHandler dependencies.
type AgentDeps struct {
	Agent      *aiagent.Agent
	Store      *aiagent.Store
	Approver   *aiagent.Approver
	UsernameOf func(uint) string

	// PolicyGet / PolicySet read and write the per-tool policy overrides.
	PolicyGet func() map[string]string
	PolicySet func(map[string]string) error

	// MCPStatus reports the state of the outbound MCP connections (M8-C).
	MCPStatus func() []aiagent.MCPStatus
	// MCPServerToken returns the token that protects the inbound MCP endpoint (M8-F).
	MCPServerToken func() string
	Settings       Settings
	// DataDir returns the panel data directory (holds the archived reports).
	DataDir func() string
}

// AgentHandler serves the AI Agent endpoints: streaming ReAct chat, the
// human-in-the-loop approval callback, session history and tool inventory.
type AgentHandler struct {
	deps AgentDeps
}

// NewAgentHandler constructs an AgentHandler.
func NewAgentHandler(deps AgentDeps) *AgentHandler {
	return &AgentHandler{deps: deps}
}

// agentChatReq is the agent chat request body.
type agentChatReq struct {
	SessionID string `json:"session_id"`
	Message   string `json:"message" binding:"required"`
	Context   string `json:"context"`
	DeepThink bool   `json:"deep_think"`
}

// approveReq is the approval decision body.
type approveReq struct {
	ApprovalID string `json:"approval_id" binding:"required"`
	Approved   bool   `json:"approved"`
}

// contextOf reads the authenticated identity from the Gin context.
func (h *AgentHandler) contextOf(c *gin.Context) (uint, string, string) {
	var uid uint
	if v, ok := c.Get("userID"); ok {
		if id, ok := v.(uint); ok {
			uid = id
		}
	}
	var role string
	if v, ok := c.Get("role"); ok {
		if r, ok := v.(string); ok {
			role = r
		}
	}
	name := ""
	if h.deps.UsernameOf != nil && uid > 0 {
		name = h.deps.UsernameOf(uid)
	}
	return uid, role, name
}

// sseWriter prepares an SSE response and returns the event sink.
func sseWriter(c *gin.Context) (func(aiagent.Event), bool) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		response.Error(c, 500, response.CodeInternal, "streaming unsupported")
		return nil, false
	}
	return func(e aiagent.Event) {
		data, err := json.Marshal(e)
		if err != nil {
			return
		}
		if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
			return
		}
		flusher.Flush()
	}, true
}

// Chat POST /ai/agent/chat - streaming ReAct conversation (SSE).
func (h *AgentHandler) Chat(c *gin.Context) {
	var req agentChatReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	sink, ok := sseWriter(c)
	if !ok {
		return
	}
	uid, role, name := h.contextOf(c)
	opt := aiagent.RunOptions{
		SessionID: req.SessionID,
		UserID:    uid,
		Username:  name,
		RoleName:  role,
		Context:   req.Context,
		DeepThink: req.DeepThink,
		Emitter:   aiagent.EmitterFunc(sink),
	}
	res, err := h.deps.Agent.Send(c.Request.Context(), opt, req.Message)
	if err != nil {
		sessionID := req.SessionID
		if res != nil {
			sessionID = res.SessionID
		}
		sink(aiagent.Event{Type: aiagent.EventError, SessionID: sessionID, Message: err.Error()})
	}
	done := aiagent.Event{Type: aiagent.EventDone}
	if res != nil {
		done.SessionID = res.SessionID
		done.Pending = res.Suspended
	}
	sink(done)
}

// Approve POST /ai/agent/approve - resume a suspended run (SSE).
func (h *AgentHandler) Approve(c *gin.Context) {
	var req approveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	sink, ok := sseWriter(c)
	if !ok {
		return
	}
	uid, role, name := h.contextOf(c)
	opt := aiagent.RunOptions{
		UserID:   uid,
		Username: name,
		RoleName: role,
		Emitter:  aiagent.EmitterFunc(sink),
	}
	res, err := h.deps.Agent.Resume(c.Request.Context(), opt, req.ApprovalID, req.Approved)
	if err != nil {
		sessionID := ""
		if res != nil {
			sessionID = res.SessionID
		}
		sink(aiagent.Event{Type: aiagent.EventError, SessionID: sessionID, Message: err.Error()})
	}
	done := aiagent.Event{Type: aiagent.EventDone}
	if res != nil {
		done.SessionID = res.SessionID
		done.Pending = res.Suspended
	}
	sink(done)
}

// Sessions GET /ai/agent/sessions
func (h *AgentHandler) Sessions(c *gin.Context) {
	uid, _, _ := h.contextOf(c)
	list, err := h.deps.Store.ListSessions(uid, 100)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"sessions": list})
}

// SessionMessages GET /ai/agent/sessions/:id/messages
func (h *AgentHandler) SessionMessages(c *gin.Context) {
	uid, _, _ := h.contextOf(c)
	id := c.Param("id")
	sess, err := h.deps.Store.GetSession(id, uid)
	if err != nil {
		response.Error(c, 404, response.CodeNotFound, err.Error())
		return
	}
	msgs, err := h.deps.Store.History(id, 0)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	pending, _ := h.deps.Approver.Pending(id)
	response.OK(c, gin.H{"session": sess, "messages": msgs, "pending": pending})
}

// DeleteSession DELETE /ai/agent/sessions/:id
func (h *AgentHandler) DeleteSession(c *gin.Context) {
	uid, _, _ := h.contextOf(c)
	if err := h.deps.Store.DeleteSession(c.Param("id"), uid); err != nil {
		response.Error(c, 404, response.CodeNotFound, err.Error())
		return
	}
	response.OK(c, gin.H{"deleted": true})
}

// Tools GET /ai/agent/tools - the tool inventory with risk levels and policies.
func (h *AgentHandler) Tools(c *gin.Context) {
	_, role, _ := h.contextOf(c)
	pol := map[string]string{}
	if h.deps.PolicyGet != nil {
		pol = h.deps.PolicyGet()
	}
	type toolView struct {
		Name        string `json:"name"`
		Title       string `json:"title"`
		Description string `json:"description"`
		Risk        string `json:"risk"`
		Perm        string `json:"perm"`
		Source      string `json:"source"`
		Policy      string `json:"policy"`
	}
	out := make([]toolView, 0, h.deps.Agent.Registry().Len())
	for _, t := range h.deps.Agent.Registry().List() {
		p := pol[t.Name]
		if p == "" {
			p = aiagent.PolicyFor(t, "").String()
		}
		if t.Risk == aiagent.RiskForbidden {
			p = aiagent.PolicyDeny.String()
		}
		out = append(out, toolView{
			Name: t.Name, Title: t.Title, Description: t.Description,
			Risk: t.Risk.String(), Perm: t.Perm, Source: t.Source, Policy: p,
		})
	}
	response.OK(c, gin.H{"tools": out, "role": role, "mcp": h.statusOfMCP()})
}

// statusOfMCP returns the MCP connection status (empty when MCP is not wired).
func (h *AgentHandler) statusOfMCP() []aiagent.MCPStatus {
	if h.deps.MCPStatus == nil {
		return nil
	}
	return h.deps.MCPStatus()
}

// toolPolicyReq is the per-tool policy update body.
type toolPolicyReq struct {
	Policies map[string]string `json:"policies"`
}

// SetToolPolicy PUT /ai/agent/tools/policy
func (h *AgentHandler) SetToolPolicy(c *gin.Context) {
	var req toolPolicyReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	current := map[string]string{}
	if h.deps.PolicyGet != nil {
		current = h.deps.PolicyGet()
	}
	for name, policy := range req.Policies {
		t, ok := h.deps.Agent.Registry().Get(name)
		if !ok {
			response.Error(c, 400, response.CodeInvalidParam, "未知工具："+name)
			return
		}
		switch policy {
		case "auto":
			// High-risk and forbidden tools can never run unattended.
			if t.Risk == aiagent.RiskHighWrite || t.Risk == aiagent.RiskForbidden {
				response.Error(c, 400, response.CodeInvalidParam,
					fmt.Sprintf("工具 %s 属于高危操作，不允许设置为免确认", name))
				return
			}
			current[name] = "auto"
		case "confirm", "deny":
			current[name] = policy
		case "":
			delete(current, name)
		default:
			response.Error(c, 400, response.CodeInvalidParam, "无效策略："+policy)
			return
		}
	}
	if h.deps.PolicySet != nil {
		if err := h.deps.PolicySet(current); err != nil {
			response.Error(c, 500, response.CodeInternal, err.Error())
			return
		}
	}
	response.OK(c, gin.H{"policies": current})
}

// ResetSession DELETE /ai/agent/messages - clear the transcript of a session
// without deleting the session itself.
func (h *AgentHandler) ResetSession(c *gin.Context) {
	uid, _, _ := h.contextOf(c)
	id := strings.TrimSpace(c.Query("session_id"))
	if id == "" {
		response.Error(c, 400, response.CodeInvalidParam, "缺少 session_id")
		return
	}
	if _, err := h.deps.Store.GetSession(id, uid); err != nil {
		response.Error(c, 404, response.CodeNotFound, err.Error())
		return
	}
	if err := h.deps.Store.Repository().DeleteSession(id); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"reset": true})
}

// ConfigInfo GET /ai/agent/config - the runtime agent configuration and health.
func (h *AgentHandler) ConfigInfo(c *gin.Context) {
	modelReady := h.deps.Agent.LLM().Validate() == nil
	response.OK(c, gin.H{
		"max_steps":            h.deps.Agent.MaxSteps(),
		"approval_ttl_seconds": int(h.deps.Approver.TTL().Seconds()),
		"tool_count":           h.deps.Agent.Registry().Len(),
		"model_ready":          modelReady,
	})
}

// mcpServerState reads the inbound MCP server configuration from the panel settings.
func (h *AgentHandler) mcpServerState() (enabled, allowHigh bool) {
	enabled = true // exposed by default, but always protected by a bearer token
	allowHigh = false
	if h.deps.Settings.Get != nil {
		if v := h.deps.Settings.Get("mcp_server_enabled"); v == "false" || v == "0" {
			enabled = false
		}
		if v := h.deps.Settings.Get("mcp_server_allow_high_risk"); v == "true" || v == "1" {
			allowHigh = true
		}
	}
	return enabled, allowHigh
}

// MCPServerInfo GET /ai/agent/mcp-server - the inbound MCP endpoint config (M8-F).
func (h *AgentHandler) MCPServerInfo(c *gin.Context) {
	enabled, allowHigh := h.mcpServerState()
	token := ""
	if h.deps.MCPServerToken != nil {
		token = h.deps.MCPServerToken()
	}
	highRisk := 0
	for _, t := range h.deps.Agent.Registry().List() {
		if t.Risk == aiagent.RiskHighWrite {
			highRisk++
		}
	}
	response.OK(c, gin.H{
		"enabled":              enabled,
		"allow_high_risk":      allowHigh,
		"token":                token,
		"endpoint":             "/mcp",
		"high_risk_tool_count": highRisk,
	})
}

// mcpServerUpdateReq is the inbound MCP endpoint update body.
type mcpServerUpdateReq struct {
	Enabled       *bool `json:"enabled"`
	AllowHighRisk *bool `json:"allow_high_risk"`
	RotateToken   bool  `json:"rotate_token"`
}

// UpdateMCPServer PUT /ai/agent/mcp-server
func (h *AgentHandler) UpdateMCPServer(c *gin.Context) {
	var req mcpServerUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Error(c, 400, response.CodeInvalidParam, err.Error())
		return
	}
	if h.deps.Settings.Set == nil {
		response.Error(c, 500, response.CodeInternal, "设置服务不可用")
		return
	}
	kv := map[string]string{}
	if req.Enabled != nil {
		kv["mcp_server_enabled"] = boolText(*req.Enabled)
	}
	if req.AllowHighRisk != nil {
		kv["mcp_server_allow_high_risk"] = boolText(*req.AllowHighRisk)
	}
	if req.RotateToken {
		token, err := newSecretToken()
		if err != nil {
			response.Error(c, 500, response.CodeInternal, err.Error())
			return
		}
		kv["mcp_server_token"] = token
	}
	if len(kv) == 0 {
		response.Error(c, 400, response.CodeInvalidParam, "没有需要更新的字段")
		return
	}
	if err := h.deps.Settings.Set(kv); err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	h.MCPServerInfo(c)
}

// boolText renders a boolean as the setting value stored in the database.
func boolText(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// newSecretToken generates a random hex token for the MCP endpoint.
func newSecretToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// SessionReport GET /ai/agent/sessions/:id/report - download the Markdown report
// of a session (works for scheduled task reports and for ad-hoc conversations).
func (h *AgentHandler) SessionReport(c *gin.Context) {
	uid, _, _ := h.contextOf(c)
	id := c.Param("id")
	sess, err := h.deps.Store.GetSession(id, uid)
	if err != nil {
		response.Error(c, 404, response.CodeNotFound, err.Error())
		return
	}
	msgs, err := h.deps.Store.History(id, 0)
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	steps, tools := 0, 0
	for _, m := range msgs {
		if m.Role != "assistant" {
			continue
		}
		steps++
		tools += len(m.ToolCalls)
	}
	md, err := h.deps.Store.BuildReport(id, aiagent.ReportMeta{
		Prompt: firstUserMessage(msgs),
		Steps:  steps,
		Tools:  tools,
		Status: "success",
	})
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	// Scheduled tasks already embed the date in the title ("每日巡检 · 2026-10-16"),
	// so the title alone makes a unique filename; only ad-hoc sessions fall back
	// to the download date.
	filename := sanitizeReportName(sess.Title)
	if hasDateSuffix(sess.Title) {
		filename += ".md"
	} else {
		filename += "-" + time.Now().Format("20060102") + ".md"
	}
	writeMarkdownDownload(c, filename, md)
}

// Reports GET /ai/agent/reports - list the archived AI task reports.
func (h *AgentHandler) Reports(c *gin.Context) {
	files, err := aiagent.ListReports(h.dataDir())
	if err != nil {
		response.Error(c, 500, response.CodeInternal, err.Error())
		return
	}
	response.OK(c, gin.H{"reports": files})
}

// DownloadReport GET /ai/agent/reports/download?name=... - download an archived report.
func (h *AgentHandler) DownloadReport(c *gin.Context) {
	name := c.Query("name")
	if name == "" {
		response.Error(c, 400, response.CodeInvalidParam, "缺少 name 参数")
		return
	}
	data, base, err := aiagent.ReadReport(h.dataDir(), name)
	if err != nil {
		response.Error(c, 404, response.CodeNotFound, err.Error())
		return
	}
	writeMarkdownDownload(c, base, string(data))
}

// dataDir resolves the panel data directory.
func (h *AgentHandler) dataDir() string {
	if h.deps.DataDir == nil {
		return ""
	}
	return h.deps.DataDir()
}

// firstUserMessage returns the first user message of a transcript.
func firstUserMessage(msgs []aiagent.Message) string {
	for _, m := range msgs {
		if m.Role == "user" {
			return m.Content
		}
	}
	return ""
}

// hasDateSuffix reports whether the title already ends with a YYYY-MM-DD date,
// which scheduled task titles do ("每日主机巡检 · 2026-10-16").
func hasDateSuffix(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) < 10 {
		return false
	}
	tail := s[len(s)-10:]
	// Expect 2026-10-16.
	if tail[4] != '-' || tail[7] != '-' {
		return false
	}
	for i, ch := range tail {
		if i == 4 || i == 7 {
			continue
		}
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return true
}

// sanitizeReportName keeps a report filename free of path separators and control
// characters (task names are usually Chinese, which is fine).
func sanitizeReportName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("/", "-", "\\", "-", ":", "-", "*", "-", "?", "-", "\"", "-",
		"<", "-", ">", "-", "|", "-", "\n", " ", "\r", " ", "\t", " ").Replace(s)
	if s == "" {
		s = "report"
	}
	if len([]rune(s)) > 60 {
		s = string([]rune(s)[:60])
	}
	return s
}

// writeMarkdownDownload sends a Markdown file as an attachment. The filename is
// RFC 5987 encoded so non-ASCII task names survive.
func writeMarkdownDownload(c *gin.Context, filename, content string) {
	if filename == "" {
		filename = "report.md"
	}
	c.Header("Content-Type", "text/markdown; charset=utf-8")
	c.Header("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(filename))
	c.Header("Cache-Control", "no-store")
	c.String(http.StatusOK, content)
}
