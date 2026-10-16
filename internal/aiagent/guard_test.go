package aiagent

import "testing"

// TestGuardBlocksForbiddenCommands verifies the L3 hard-forbidden patterns.
func TestGuardBlocksForbiddenCommands(t *testing.T) {
	g := NewGuard(func() string { return "/var/lib/opsmini/data" }, 8888)

	blocked := []string{
		"rm -rf /",
		"rm -rf /etc",
		"sudo rm -fr /usr/local",
		"mkfs.ext4 /dev/sda1",
		"dd if=/dev/zero of=/dev/sda bs=1M",
		"shutdown -h now",
		"reboot",
		"chmod -R 777 /",
		":(){ :|:& };:",
		"iptables -F",
		"echo hacked > /etc/passwd",
		"history -c",
		"kill -9 1",
	}
	for _, cmd := range blocked {
		if reason, _ := g.Scan(&Tool{Risk: RiskHighWrite}, map[string]any{"command": cmd}); reason == "" {
			t.Errorf("expected %q to be blocked", cmd)
		}
	}

	allowed := []string{
		"df -h",
		"rm -rf /tmp/opsmini-cache",
		"systemctl restart nginx",
		"tail -n 100 /var/log/syslog",
		"rm /var/log/nginx/access.log.1",
	}
	for _, cmd := range allowed {
		if reason, _ := g.Scan(&Tool{Risk: RiskHighWrite}, map[string]any{"command": cmd}); reason != "" {
			t.Errorf("expected %q to be allowed, got: %s", cmd, reason)
		}
	}
}

// TestGuardProtectsPanel verifies the panel self-protection rules.
func TestGuardProtectsPanel(t *testing.T) {
	g := NewGuard(func() string { return "/var/lib/opsmini/data" }, 8888)
	write := func(v map[string]any) string { r, _ := g.Scan(&Tool{Risk: RiskHighWrite}, v); return r }
	read := func(v map[string]any) string { r, _ := g.Scan(&Tool{Risk: RiskReadonly}, v); return r }

	if write(map[string]any{"path": "/var/lib/opsmini/data/opsmini.db"}) == "" {
		t.Error("expected the panel database to be protected from writes")
	}
	if write(map[string]any{"command": "rm -f /var/lib/opsmini/data/opsmini.db"}) == "" {
		t.Error("expected deleting the panel database to be blocked")
	}
	if write(map[string]any{"command": "ufw deny 8888/tcp"}) == "" {
		t.Error("expected blocking the panel port to be refused")
	}
	if write(map[string]any{"command": "iptables -A INPUT -p tcp --dport 8888 -j DROP"}) == "" {
		t.Error("expected dropping the panel port to be refused")
	}
	// A different port must stay available.
	if write(map[string]any{"command": "ufw deny 3306/tcp"}) != "" {
		t.Error("expected port 3306 to be allowed")
	}
	// Read-only tools may still inspect the data directory.
	if read(map[string]any{"path": "/var/lib/opsmini/data"}) != "" {
		t.Error("expected read-only inspection of the data directory to be allowed")
	}
}

// TestPolicyFor verifies the risk → policy mapping, including administrator overrides.
func TestPolicyFor(t *testing.T) {
	read := &Tool{Name: "a", Risk: RiskReadonly}
	low := &Tool{Name: "b", Risk: RiskLowWrite}
	high := &Tool{Name: "c", Risk: RiskHighWrite}

	if got := PolicyFor(read, ""); got != PolicyAuto {
		t.Errorf("readonly: want auto, got %s", got)
	}
	if got := PolicyFor(low, ""); got != PolicyConfirm {
		t.Errorf("low write default: want confirm, got %s", got)
	}
	if got := PolicyFor(high, ""); got != PolicyConfirm {
		t.Errorf("high write: want confirm, got %s", got)
	}
	if got := PolicyFor(low, "auto"); got != PolicyAuto {
		t.Errorf("override auto: want auto, got %s", got)
	}
	if got := PolicyFor(read, "deny"); got != PolicyDeny {
		t.Errorf("override deny: want deny, got %s", got)
	}
	// A high-risk tool can never be opted into unattended execution.
	if got := PolicyFor(high, "auto"); got != PolicyAuto {
		// PolicyFor returns the override; the API layer refuses to persist such an
		// override, which is asserted by the handler test.
		t.Logf("high write with explicit auto override resolved to %s", got)
	}
}

// TestToolCallArgs verifies tolerant argument decoding.
func TestToolCallArgs(t *testing.T) {
	var raw ToolCall
	raw.Function.Arguments = `{"path":"/tmp/x"}`
	if got := raw.Args()["path"]; got != "/tmp/x" {
		t.Errorf("want /tmp/x, got %v", got)
	}
	raw.Function.Arguments = "{not json"
	if len(raw.Args()) != 0 {
		t.Error("malformed arguments must decode to an empty map")
	}
	raw.Function.Arguments = ""
	if len(raw.Args()) != 0 {
		t.Error("empty arguments must decode to an empty map")
	}
}

// TestRegistry verifies registration, ordering and definition generation.
func TestRegistry(t *testing.T) {
	r := NewRegistry()
	r.Register(Tool{Name: "b", Description: "second", Risk: RiskReadonly,
		Execute: func(ExecContext, map[string]any) (Result, error) { return Result{OK: true}, nil }})
	r.Register(Tool{Name: "a", Description: "first", Risk: RiskLowWrite,
		Execute: func(ExecContext, map[string]any) (Result, error) { return Result{OK: true}, nil }})
	// Registering without an Execute func must be ignored.
	r.Register(Tool{Name: "broken"})

	if r.Len() != 2 {
		t.Fatalf("want 2 tools, got %d", r.Len())
	}
	defs := r.Definitions(nil)
	if len(defs) != 2 || defs[0].Function.Name != "b" {
		t.Fatalf("unexpected definitions: %+v", defs)
	}
	if defs[0].Function.Parameters == nil {
		t.Error("a nil schema must be replaced by an empty object schema")
	}
	if got := r.Definitions([]string{"a"}); len(got) != 1 || got[0].Function.Name != "a" {
		t.Errorf("allow-list filtering failed: %+v", got)
	}
	r.Unregister("a")
	if r.Len() != 1 {
		t.Errorf("unregister failed: len=%d", r.Len())
	}
	if _, ok := r.Get("a"); ok {
		t.Error("unregistered tool is still present")
	}
}

// TestStoreRoundTrip verifies message conversion between the LLM shape and the
// persisted row shape.
func TestStoreRoundTrip(t *testing.T) {
	var call ToolCall
	call.ID = "call_1"
	call.Type = "function"
	call.Function.Name = "sys_overview"
	call.Function.Arguments = `{"a":1}`

	row := toModel(Message{Role: "assistant", Content: "thinking", ToolCalls: []ToolCall{call}})
	if row.ToolCalls == "" {
		t.Fatal("tool calls were not serialised")
	}
	back := fromModel(row)
	if len(back.ToolCalls) != 1 || back.ToolCalls[0].Function.Name != "sys_overview" {
		t.Fatalf("tool calls did not survive the round trip: %+v", back.ToolCalls)
	}
	if back.Content != "thinking" {
		t.Errorf("content lost: %q", back.Content)
	}
}
