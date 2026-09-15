package agent

import (
	"bytes"
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	agentpb "github.com/opsmini/opsmini/internal/agent/pb"
)

// TestServerExecute is an end-to-end test: a real Server accepts an Agent,
// then Execute() dispatches a command and receives the TaskResult.
func TestServerExecute(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := NewServer("secret")
	go srv.Serve(lis)

	c := New(Config{
		ServerAddr: lis.Addr().String(),
		Token:      "secret",
		Heartbeat:  time.Hour,
	}, "testhost", "test")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	// Wait for the Agent to register.
	deadline := time.Now().Add(3 * time.Second)
	for srv.Agent("testhost") == nil {
		if time.Now().After(deadline) {
			t.Fatal("agent not registered in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Dispatch an Execute task.
	result, err := srv.Execute(context.Background(), "testhost", &agentpb.ExecuteTask{
		JobId: "job-1",
		Kind:  &agentpb.ExecuteTask_Command{Command: &agentpb.CommandSpec{Command: "echo from-server"}},
	})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if result.JobId != "job-1" || result.Status != "success" || result.Stdout != "from-server\n" {
		t.Fatalf("unexpected result: %+v", result)
	}

	// Agents() should list the host.
	if len(srv.Agents()) != 1 || srv.Agents()[0] != "testhost" {
		t.Fatalf("Agents() = %v, want [testhost]", srv.Agents())
	}
}

// TestServerRejectsBadToken verifies registration auth when a token is configured.
func TestServerRejectsBadToken(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := NewServer("right-token")
	go srv.Serve(lis)

	// Agent with a wrong token must not register.
	c := New(Config{ServerAddr: lis.Addr().String(), Token: "wrong-token", Heartbeat: time.Hour}, "badhost", "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	time.Sleep(300 * time.Millisecond)
	if srv.Agent("badhost") != nil {
		t.Fatal("agent registered despite wrong token")
	}
}

// TestServerPullFile verifies the Server can pull a file from an Agent.
func TestServerPullFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	content := []byte("hello pull file\n")
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := NewServer("secret")
	go srv.Serve(lis)

	c := New(Config{ServerAddr: lis.Addr().String(), Token: "secret", Heartbeat: time.Hour}, "pullhost", "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for srv.Agent("pullhost") == nil {
		if time.Now().After(deadline) {
			t.Fatal("agent not registered in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	data, err := srv.PullFile(context.Background(), "pullhost", src)
	if err != nil {
		t.Fatalf("pull: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("content mismatch: got %q want %q", data, content)
	}
}

// TestServerPushFile verifies the Server can push a (multi-chunk) file to an Agent.
func TestServerPushFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	content := bytes.Repeat([]byte("0123456789abcdef"), 16*1024) // 256 KB, spans multiple 64 KiB chunks
	if err := os.WriteFile(src, content, 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	dest := filepath.Join(dir, "dest.bin")

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := NewServer("secret")
	go srv.Serve(lis)

	c := New(Config{ServerAddr: lis.Addr().String(), Token: "secret", Heartbeat: time.Hour}, "pushhost", "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for srv.Agent("pushhost") == nil {
		if time.Now().After(deadline) {
			t.Fatal("agent not registered in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := srv.PushFile(context.Background(), "pushhost", src, dest, "0755"); err != nil {
		t.Fatalf("push: %v", err)
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("read dest: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("content mismatch: got %d bytes, want %d bytes", len(got), len(content))
	}
	if st, err := os.Stat(dest); err == nil && st.Mode().Perm() != 0o755 {
		t.Fatalf("mode = %o, want 755", st.Mode().Perm())
	}
}

// TestServerStatus verifies AgentStatuses reports online status after registration.
func TestServerStatus(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := NewServer("secret")
	go srv.Serve(lis)

	c := New(Config{ServerAddr: lis.Addr().String(), Token: "secret", Heartbeat: time.Hour}, "statushost", "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go c.Run(ctx)

	deadline := time.Now().Add(3 * time.Second)
	for srv.Agent("statushost") == nil {
		if time.Now().After(deadline) {
			t.Fatal("agent not registered in time")
		}
		time.Sleep(10 * time.Millisecond)
	}

	statuses := srv.AgentStatuses()
	if len(statuses) != 1 {
		t.Fatalf("AgentStatuses() len=%d, want 1", len(statuses))
	}
	st := statuses[0]
	if st.Hostname != "statushost" || !st.Online {
		t.Fatalf("unexpected status: %+v", st)
	}
}
