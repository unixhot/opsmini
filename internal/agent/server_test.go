package agent

import (
	"context"
	"net"
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
