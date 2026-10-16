package aiagent

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPServer exposes the agent's tool registry over the Model Context Protocol
// (Streamable HTTP), so external MCP clients — Claude Desktop, Cursor, OpsAnt or
// another OpsMini — can drive the host with exactly the same tools the built-in
// agent uses. This is the reverse direction of MCPClientManager, and it comes for
// free because tools are declared with plain JSON Schema.
type MCPServer struct {
	reg     *Registry
	guard   *Guard
	version string

	// Token returns the bearer token required from MCP clients (lazily, so it can
	// be rotated at runtime).
	Token func() string
	// Allowed reports whether the MCP endpoint is enabled at all.
	Allowed func() bool
	// AllowHighRisk reports whether L2 tools are exposed over MCP. When false
	// (the default) high-risk tools are not published at all, because an external
	// MCP client cannot present OpsMini's confirmation dialog.
	AllowHighRisk func() bool
	// Audit records every remote tool invocation.
	Audit AuditFunc

	mu      sync.Mutex
	built   *mcp.Server
	builtN  int
	builtHi bool
}

// NewMCPServer creates an MCP server facade over the tool registry.
func NewMCPServer(reg *Registry, guard *Guard, version string) *MCPServer {
	return &MCPServer{reg: reg, guard: guard, version: version}
}

// Handler returns the HTTP handler to mount at the MCP endpoint.
func (s *MCPServer) Handler() http.Handler {
	inner := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s.server()
	}, &mcp.StreamableHTTPOptions{Stateless: true})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Allowed != nil && !s.Allowed() {
			http.Error(w, "OpsMini MCP server is disabled", http.StatusForbidden)
			return
		}
		if !s.authorized(r) {
			w.Header().Set("WWW-Authenticate", `Bearer realm="opsmini-mcp"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		inner.ServeHTTP(w, r)
	})
}

// authorized validates the bearer token from the Authorization header or the
// token query parameter (some MCP clients cannot set custom headers).
func (s *MCPServer) authorized(r *http.Request) bool {
	want := ""
	if s.Token != nil {
		want = s.Token()
	}
	if want == "" {
		return false
	}
	if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
		if strings.TrimSpace(strings.TrimPrefix(auth, "Bearer ")) == want {
			return true
		}
	}
	if t := r.URL.Query().Get("token"); t != "" && t == want {
		return true
	}
	return false
}

// server builds (and caches) the MCP server exposing the current tool set.
func (s *MCPServer) server() *mcp.Server {
	s.mu.Lock()
	defer s.mu.Unlock()

	tools := s.reg.List()
	allowHigh := s.AllowHighRisk != nil && s.AllowHighRisk()
	if s.built != nil && s.builtN == len(tools) && s.builtHi == allowHigh {
		return s.built
	}

	srv := mcp.NewServer(&mcp.Implementation{
		Name:        "opsmini",
		Title:       "OpsMini",
		Description: "OpsMini 主机运维面板：系统信息、进程、端口、磁盘、日志、容器、安全与文件操作工具集",
		Version:     s.version,
	}, nil)

	for _, t := range tools {
		tool := t
		// L3 patterns are never published; L2 stays hidden until the administrator
		// explicitly opts in.
		if tool.Risk == RiskForbidden {
			continue
		}
		if tool.Risk == RiskHighWrite && !allowHigh {
			continue
		}
		inputSchema := tool.Schema
		if inputSchema == nil {
			inputSchema = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		annotations := &mcp.ToolAnnotations{Title: tool.Title}
		if tool.Risk == RiskReadonly {
			annotations.ReadOnlyHint = true
		} else {
			annotations.DestructiveHint = boolPointer(tool.Risk == RiskHighWrite)
		}
		srv.AddTool(&mcp.Tool{
			Name:        tool.Name,
			Title:       tool.Title,
			Description: tool.Description,
			InputSchema: inputSchema,
			Annotations: annotations,
		}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return s.call(ctx, tool, req)
		})
	}

	s.built = srv
	s.builtN = len(tools)
	s.builtHi = allowHigh
	return srv
}

// call executes a tool on behalf of an external MCP client.
func (s *MCPServer) call(ctx context.Context, t *Tool, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args := map[string]any{}
	if req != nil && req.Params != nil && len(req.Params.Arguments) > 0 {
		if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
			return mcpError("参数解析失败：" + err.Error()), nil
		}
	}

	// The guard rails apply to remote callers exactly as they do to the agent.
	if s.guard != nil {
		if reason, rule := s.guard.Scan(t, args); reason != "" {
			if s.Audit != nil {
				s.Audit(0, "mcp-client", "mcp_tool", t.Name, "deny:"+rule+": "+t.Name, "")
			}
			return mcpError(reason), nil
		}
	}
	if t.Risk == RiskHighWrite && (s.AllowHighRisk == nil || !s.AllowHighRisk()) {
		return mcpError("该工具属于高危操作，MCP 远程调用默认被禁用（可在面板中开启「允许 MCP 高危操作」）"), nil
	}

	result, err := t.Execute(ExecContext{Context: ctx, Username: "mcp-client"}, args)
	if s.Audit != nil {
		detail := t.Name + " " + string(req.Params.Arguments)
		if err != nil {
			detail += " | error: " + err.Error()
		}
		s.Audit(0, "mcp-client", "mcp_tool", t.Name, truncateText(detail, 500), "")
	}
	if err != nil {
		return mcpError("工具执行失败：" + err.Error()), nil
	}
	text := result.Summary
	if result.Detail != "" && result.Detail != result.Summary {
		text = result.Summary + "\n\n" + result.Detail
	}
	return &mcp.CallToolResult{
		IsError: !result.OK,
		Content: []mcp.Content{&mcp.TextContent{Text: text}},
	}, nil
}

// mcpError builds an error tool result.
func mcpError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: msg}}}
}

// boolPointer returns a pointer to v.
func boolPointer(v bool) *bool { return &v }
