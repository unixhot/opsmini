package aiagent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// newTestTool builds a minimal tool for the MCP tests.
func newTestTool(name string, risk RiskLevel, run func() Result) Tool {
	return Tool{
		Name:        name,
		Title:       name,
		Description: "test tool " + name,
		Risk:        risk,
		Schema:      map[string]any{"type": "object", "properties": map[string]any{}},
		Execute: func(ExecContext, map[string]any) (Result, error) {
			if run == nil {
				return Result{OK: true, Summary: "ok"}, nil
			}
			return run(), nil
		},
	}
}

// TestMCPServerPublishesTools verifies that the inbound MCP endpoint publishes
// the registry tools, hides high-risk ones by default, and bridges a remote call
// through to the local tool.
func TestMCPServerPublishesTools(t *testing.T) {
	reg := NewRegistry()
	calls := 0
	reg.Register(newTestTool("hello", RiskReadonly, func() Result {
		calls++
		return Result{OK: true, Summary: "hello from opsmini"}
	}))
	reg.Register(newTestTool("break_things", RiskHighWrite, nil))

	srv := NewMCPServer(reg, NewGuard(nil, 8888), "test")
	srv.Token = func() string { return "secret" }
	srv.Allowed = func() bool { return true }
	srv.AllowHighRisk = func() bool { return false }

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := srv.server().Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer session.Close()

	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(list.Tools) != 1 || list.Tools[0].Name != "hello" {
		names := make([]string, 0, len(list.Tools))
		for _, tl := range list.Tools {
			names = append(names, tl.Name)
		}
		t.Fatalf("want only the read-only tool, got %v", names)
	}
	if list.Tools[0].Annotations == nil || !list.Tools[0].Annotations.ReadOnlyHint {
		t.Error("read-only tools must carry the readOnlyHint annotation")
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{Name: "hello"})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if res.IsError {
		t.Fatalf("call reported an error: %+v", res.Content)
	}
	if calls != 1 {
		t.Errorf("want the local tool to run once, ran %d times", calls)
	}
	if text := mcpContentText(res); !strings.Contains(text, "hello from opsmini") {
		t.Errorf("unexpected result text: %q", text)
	}
}

// TestMCPServerExposesHighRiskOnOptIn verifies that L2 tools become available
// (and still guarded) when the administrator opts in.
func TestMCPServerExposesHighRiskOnOptIn(t *testing.T) {
	reg := NewRegistry()
	reg.Register(newTestTool("exec_command", RiskHighWrite, nil))
	reg.Register(newTestTool("list_ports", RiskReadonly, nil))

	srv := NewMCPServer(reg, NewGuard(nil, 8888), "test")
	srv.Token = func() string { return "secret" }
	srv.Allowed = func() bool { return true }
	srv.AllowHighRisk = func() bool { return true }

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := srv.server().Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer session.Close()

	list, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(list.Tools) != 2 {
		t.Fatalf("want 2 tools with high risk enabled, got %d", len(list.Tools))
	}
}

// TestMCPServerGuardBlocksRemoteCalls verifies that the L3 rails apply to calls
// coming from an external MCP client.
func TestMCPServerGuardBlocksRemoteCalls(t *testing.T) {
	reg := NewRegistry()
	ran := false
	reg.Register(Tool{
		Name:        "exec_command",
		Description: "run a command",
		Risk:        RiskHighWrite,
		Schema: map[string]any{
			"type":       "object",
			"properties": map[string]any{"command": map[string]any{"type": "string"}},
		},
		Execute: func(ExecContext, map[string]any) (Result, error) {
			ran = true
			return Result{OK: true, Summary: "should not happen"}, nil
		},
	})

	srv := NewMCPServer(reg, NewGuard(nil, 8888), "test")
	srv.Token = func() string { return "secret" }
	srv.Allowed = func() bool { return true }
	srv.AllowHighRisk = func() bool { return true }

	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	if _, err := srv.server().Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("connect server: %v", err)
	}
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("connect client: %v", err)
	}
	defer session.Close()

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "exec_command",
		Arguments: map[string]any{"command": "rm -rf /"},
	})
	if err != nil {
		t.Fatalf("call tool: %v", err)
	}
	if !res.IsError {
		t.Error("a forbidden command must fail over MCP")
	}
	if ran {
		t.Error("the forbidden command reached the tool implementation")
	}
	if text := mcpContentText(res); !strings.Contains(text, "硬禁止") {
		t.Errorf("unexpected refusal text: %q", text)
	}
}

// TestMCPAuthorized verifies the bearer token and query parameter auth paths.
func TestMCPAuthorized(t *testing.T) {
	srv := NewMCPServer(NewRegistry(), nil, "test")
	srv.Token = func() string { return "tok" }

	req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
	if srv.authorized(req) {
		t.Error("a request without credentials must be rejected")
	}
	req = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer tok")
	if !srv.authorized(req) {
		t.Error("a valid bearer token must be accepted")
	}
	req = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	if srv.authorized(req) {
		t.Error("a wrong bearer token must be rejected")
	}
	req = httptest.NewRequest(http.MethodPost, "/mcp?token=tok", nil)
	if !srv.authorized(req) {
		t.Error("the token query parameter must be accepted")
	}
	// An empty configured token must never authorise anything.
	srv.Token = func() string { return "" }
	req = httptest.NewRequest(http.MethodPost, "/mcp", nil)
	req.Header.Set("Authorization", "Bearer ")
	if srv.authorized(req) {
		t.Error("an empty configured token must not authorise")
	}
}
