package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/opsmini/opsmini/internal/model"
)

// MCPClientManager connects the agent to the MCP (Model Context Protocol)
// servers configured in the panel and turns each remote tool into a local agent
// tool. It uses the official Go SDK, so both stdio and Streamable HTTP servers
// are supported.
//
// Remote tools are namespaced as mcp__<server>__<tool> to avoid collisions with
// the built-in tool set.
type MCPClientManager struct {
	mu       sync.Mutex
	registry *Registry
	conns    map[uint]*mcpConn
	version  string
}

// mcpConn is one live MCP server connection.
type mcpConn struct {
	cfg     model.McpServer
	session *mcp.ClientSession
	cancel  context.CancelFunc
	tools   []string // registered local tool names
}

// NewMCPClientManager creates a manager bound to a tool registry.
func NewMCPClientManager(reg *Registry, version string) *MCPClientManager {
	return &MCPClientManager{registry: reg, conns: map[uint]*mcpConn{}, version: version}
}

// Reload reconciles the live connections with the panel configuration: new and
// enabled servers are connected, removed or disabled ones are dropped, and the
// tool list of every kept server is refreshed.
func (m *MCPClientManager) Reload(servers []model.McpServer) {
	want := map[uint]model.McpServer{}
	for _, s := range servers {
		if s.Enabled {
			want[s.ID] = s
		}
	}

	m.mu.Lock()
	// Drop servers that were removed, disabled or reconfigured.
	for id, conn := range m.conns {
		next, keep := want[id]
		if keep && sameServer(conn.cfg, next) {
			continue
		}
		m.dropLocked(id)
		delete(m.conns, id)
	}
	m.mu.Unlock()

	// Connect everything that is wanted but not yet connected.
	for id, s := range want {
		m.mu.Lock()
		_, exists := m.conns[id]
		m.mu.Unlock()
		if exists {
			continue
		}
		if err := m.connect(s); err != nil {
			// A broken MCP server must never take the agent down.
			log.Printf("ai agent: connect MCP server %q failed: %v", s.Name, err)
			m.registry.UnregisterSource("mcp:" + s.Name + ":")
		}
	}
}

// connect establishes one MCP session and registers its tools.
func (m *MCPClientManager) connect(cfg model.McpServer) error {
	timeout := time.Duration(cfg.Timeout) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithCancel(context.Background())

	var transport mcp.Transport
	switch strings.ToLower(cfg.Transport) {
	case "stdio", "":
		fields := strings.Fields(cfg.Command)
		if len(fields) == 0 {
			cancel()
			return errors.New("MCP 服务未配置启动命令")
		}
		transport = &mcp.CommandTransport{Command: exec.CommandContext(ctx, fields[0], fields[1:]...)}
	case "sse", "http", "streamable", "streamable-http":
		if cfg.URL == "" {
			cancel()
			return errors.New("MCP 服务未配置服务地址")
		}
		httpClient, err := mcpHTTPClient(cfg, timeout)
		if err != nil {
			cancel()
			return err
		}
		transport = &mcp.StreamableClientTransport{
			Endpoint:             cfg.URL,
			HTTPClient:           httpClient,
			DisableStandaloneSSE: true,
		}
	default:
		cancel()
		return fmt.Errorf("不支持的 MCP 传输方式：%s", cfg.Transport)
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "opsmini",
		Title:   "OpsMini",
		Version: m.version,
	}, &mcp.ClientOptions{})

	dialCtx, dialCancel := context.WithTimeout(ctx, timeout)
	defer dialCancel()
	session, err := client.Connect(dialCtx, transport, nil)
	if err != nil {
		cancel()
		return err
	}

	conn := &mcpConn{cfg: cfg, session: session, cancel: cancel}
	if err := m.loadTools(conn); err != nil {
		_ = session.Close()
		cancel()
		return err
	}

	m.mu.Lock()
	m.conns[cfg.ID] = conn
	m.mu.Unlock()
	return nil
}

// loadTools lists the remote tools and registers them as agent tools.
func (m *MCPClientManager) loadTools(conn *mcpConn) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	res, err := conn.session.ListTools(ctx, nil)
	if err != nil {
		return err
	}
	prefix := "mcp:" + conn.cfg.Name + ":"
	m.registry.UnregisterSource(prefix)

	registered := make([]string, 0, len(res.Tools))
	for _, rt := range res.Tools {
		local := mcpToolName(conn.cfg.Name, rt.Name)
		rt := rt
		risk := RiskLowWrite
		if rt.Annotations != nil && rt.Annotations.ReadOnlyHint {
			risk = RiskReadonly
		}
		var schema map[string]any
		if s, ok := rt.InputSchema.(map[string]any); ok {
			schema = s
		}
		desc := rt.Description
		if desc == "" {
			desc = rt.Name
		}
		title := rt.Title
		if title == "" {
			title = rt.Name
		}
		m.registry.Register(Tool{
			Name:        local,
			Title:       title + "（MCP: " + conn.cfg.Name + "）",
			Description: fmt.Sprintf("[来自 MCP 服务 %s] %s", conn.cfg.Name, desc),
			Schema:      schema,
			Risk:        risk,
			Source:      prefix + rt.Name,
			Preview: func(args map[string]any) string {
				raw, _ := json.Marshal(args)
				return fmt.Sprintf("调用 MCP 服务 %s 的工具 %s：%s", conn.cfg.Name, rt.Name, string(raw))
			},
			Execute: m.makeExecutor(conn, rt.Name),
		})
		registered = append(registered, local)
	}
	conn.tools = registered
	return nil
}

// makeExecutor builds the local execution function for one remote MCP tool.
func (m *MCPClientManager) makeExecutor(conn *mcpConn, remoteName string) func(ExecContext, map[string]any) (Result, error) {
	return func(exec ExecContext, args map[string]any) (Result, error) {
		timeout := time.Duration(conn.cfg.Timeout) * time.Second
		if timeout <= 0 {
			timeout = 30 * time.Second
		}
		ctx, cancel := context.WithTimeout(exec.Context, timeout)
		defer cancel()

		res, err := conn.session.CallTool(ctx, &mcp.CallToolParams{Name: remoteName, Arguments: args})
		if err != nil {
			return Result{}, fmt.Errorf("调用 MCP 工具 %s 失败：%w", remoteName, err)
		}
		text := mcpContentText(res)
		return Result{OK: !res.IsError, Summary: text, Detail: text}, nil
	}
}

// mcpContentText flattens the content blocks of a tool result into text.
func mcpContentText(res *mcp.CallToolResult) string {
	if res == nil {
		return ""
	}
	var b strings.Builder
	for _, c := range res.Content {
		switch v := c.(type) {
		case *mcp.TextContent:
			b.WriteString(v.Text)
			b.WriteString("\n")
		default:
			if raw, err := json.Marshal(c); err == nil {
				b.Write(raw)
				b.WriteString("\n")
			}
		}
	}
	if b.Len() == 0 && res.StructuredContent != nil {
		if raw, err := json.Marshal(res.StructuredContent); err == nil {
			b.Write(raw)
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		out = "（MCP 工具返回了空结果）"
	}
	return out
}

// dropLocked closes a connection and unregisters its tools. The caller holds the lock.
func (m *MCPClientManager) dropLocked(id uint) {
	conn, ok := m.conns[id]
	if !ok {
		return
	}
	m.registry.UnregisterSource("mcp:" + conn.cfg.Name + ":")
	if conn.session != nil {
		_ = conn.session.Close()
	}
	if conn.cancel != nil {
		conn.cancel()
	}
}

// Status reports the connection state of every configured server.
type MCPStatus struct {
	ID        uint     `json:"id"`
	Name      string   `json:"name"`
	Transport string   `json:"transport"`
	Connected bool     `json:"connected"`
	ToolCount int      `json:"tool_count"`
	Tools     []string `json:"tools,omitempty"`
	Error     string   `json:"error,omitempty"`
}

// Statuses returns the current state of all configured servers.
func (m *MCPClientManager) Statuses(servers []model.McpServer) []MCPStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]MCPStatus, 0, len(servers))
	for _, s := range servers {
		st := MCPStatus{ID: s.ID, Name: s.Name, Transport: s.Transport}
		if conn, ok := m.conns[s.ID]; ok {
			st.Connected = true
			st.ToolCount = len(conn.tools)
			st.Tools = append([]string{}, conn.tools...)
		} else if s.Enabled {
			st.Error = "未连接"
		}
		out = append(out, st)
	}
	return out
}

// Close shuts every connection down.
func (m *MCPClientManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id := range m.conns {
		m.dropLocked(id)
		delete(m.conns, id)
	}
}

// sameServer reports whether the two configurations would yield the same session.
func sameServer(a, b model.McpServer) bool {
	return a.Name == b.Name && a.Transport == b.Transport &&
		a.Command == b.Command && a.URL == b.URL && a.Headers == b.Headers &&
		a.Timeout == b.Timeout && a.Enabled == b.Enabled
}

// mcpToolName builds the local tool name of a remote MCP tool.
func mcpToolName(server, tool string) string {
	return "mcp__" + sanitizeToolPart(server) + "__" + sanitizeToolPart(tool)
}

// sanitizeToolPart keeps only characters that are safe in a tool name.
func sanitizeToolPart(s string) string {
	cleaned := toolPartRe.ReplaceAllString(strings.TrimSpace(s), "_")
	if cleaned == "" {
		return "unnamed"
	}
	return cleaned
}

// toolPartRe matches characters outside the allowed tool-name alphabet.
var toolPartRe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

// mcpHeader is one custom HTTP header configured for an MCP server.
type mcpHeader struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// mcpHTTPClient builds an HTTP client that injects the configured headers.
// The client has no global timeout because Streamable HTTP sessions stay open;
// per-request deadlines are applied through the request context instead.
func mcpHTTPClient(cfg model.McpServer, _ time.Duration) (*http.Client, error) {
	headers := map[string]string{}
	if strings.TrimSpace(cfg.Headers) != "" {
		var list []mcpHeader
		if err := json.Unmarshal([]byte(cfg.Headers), &list); err != nil {
			return nil, fmt.Errorf("MCP 请求头解析失败：%w", err)
		}
		for _, h := range list {
			if strings.TrimSpace(h.Name) != "" {
				headers[h.Name] = h.Value
			}
		}
	}
	var rt http.RoundTripper = http.DefaultTransport
	if len(headers) > 0 {
		rt = &headerTransport{base: http.DefaultTransport, headers: headers}
	}
	return &http.Client{Transport: rt}, nil
}

// headerTransport injects a fixed set of headers into every request.
type headerTransport struct {
	base    http.RoundTripper
	headers map[string]string
}

// RoundTrip implements http.RoundTripper.
func (t *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for k, v := range t.headers {
		clone.Header.Set(k, v)
	}
	return t.base.RoundTrip(clone)
}
