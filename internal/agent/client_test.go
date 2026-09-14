package agent

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"

	agentpb "github.com/opsmini/opsmini/internal/agent/pb"
)

// md5sum returns the hex md5 of b.
func md5sum(b []byte) string {
	s := md5.Sum(b)
	return hex.EncodeToString(s[:])
}

// TestExecuteCommand verifies command execution produces a success result.
func TestExecuteCommand(t *testing.T) {
	c := New(Config{}, "testhost", "test")
	task := &agentpb.ExecuteTask{
		JobId: "job-cmd",
		Kind:  &agentpb.ExecuteTask_Command{Command: &agentpb.CommandSpec{Command: "echo hello"}},
	}
	res := c.execute(context.Background(), task)
	if res.Status != "success" {
		t.Fatalf("expected success, got %s (stderr=%s)", res.Status, res.Stderr)
	}
	if res.Stdout != "hello\n" {
		t.Fatalf("expected stdout 'hello\\n', got %q", res.Stdout)
	}
}

// TestExecuteScript verifies a shell script runs via a temp file.
func TestExecuteScript(t *testing.T) {
	c := New(Config{}, "testhost", "test")
	task := &agentpb.ExecuteTask{
		JobId: "job-script",
		Kind:  &agentpb.ExecuteTask_Script{Script: &agentpb.ScriptSpec{
			Script:   "echo from-script",
			Language: "shell",
		}},
	}
	res := c.execute(context.Background(), task)
	if res.Status != "success" {
		t.Fatalf("expected success, got %s (stderr=%s)", res.Status, res.Stderr)
	}
	if res.Stdout != "from-script\n" {
		t.Fatalf("expected 'from-script\\n', got %q", res.Stdout)
	}
}

// TestReceiveFile verifies chunked file reception with md5 verification and mode application.
func TestReceiveFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "f.txt")
	c := New(Config{}, "testhost", "test")

	content := []byte("hello world")
	sum := md5sum(content)

	// First chunk (offset 0, not last).
	ack := c.receiveFile(context.Background(), &agentpb.PushFileTask{
		JobId: "job-f", Path: path, Offset: 0, Chunk: content[:5], Size: int64(len(content)), Md5: sum,
	})
	if ack.Error != "" {
		t.Fatalf("chunk1 error: %s", ack.Error)
	}
	if ack.Received != 5 {
		t.Fatalf("chunk1 received=%d, want 5", ack.Received)
	}

	// Last chunk (offset 5, last=true).
	ack = c.receiveFile(context.Background(), &agentpb.PushFileTask{
		JobId: "job-f", Path: path, Offset: 5, Chunk: content[5:], Size: int64(len(content)), Md5: sum, Last: true, Mode: "0755",
	})
	if ack.Error != "" {
		t.Fatalf("last chunk error: %s", ack.Error)
	}
	if ack.Received != int64(len(content)) {
		t.Fatalf("received=%d, want %d", ack.Received, len(content))
	}
	if ack.Md5 != sum {
		t.Fatalf("md5=%s, want %s", ack.Md5, sum)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if string(data) != string(content) {
		t.Fatalf("file content %q, want %q", data, content)
	}
	if st, _ := os.Stat(path); st.Mode().Perm() != 0o755 {
		t.Fatalf("mode=%o, want 755", st.Mode().Perm())
	}
}

// TestClientStream is an end-to-end test: a mock Server on a localhost port sends an
// Execute task and asserts the Agent reports a success TaskResult back over the stream.
func TestClientStream(t *testing.T) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	ms := &mockServer{t: t, done: make(chan struct{})}
	agentpb.RegisterAgentServiceServer(srv, ms)
	go srv.Serve(lis)
	defer srv.Stop()

	c := New(Config{
		ServerAddr: lis.Addr().String(),
		Token:      "secret",
		Heartbeat:  time.Hour, // disable periodic heartbeat during test
	}, "testhost", "test")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()

	select {
	case <-ms.done:
		cancel()
		<-done
	case <-time.After(5 * time.Second):
		cancel()
		<-done
		t.Fatal("timeout waiting for stream result")
	}
}

type mockServer struct {
	agentpb.UnimplementedAgentServiceServer
	t    *testing.T
	done chan struct{}
}

func (s *mockServer) Stream(stream agentpb.AgentService_StreamServer) error {
	defer close(s.done)
	// 1. Await Register.
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	reg := msg.GetRegister()
	if reg == nil || reg.Token != "secret" {
		return errors.New("unexpected register")
	}
	// 2. Send RegisterAck.
	if err := stream.Send(&agentpb.ServerMessage{Payload: &agentpb.ServerMessage_RegisterAck{
		RegisterAck: &agentpb.RegisterAck{Ok: true, AgentId: "agent-1"},
	}}); err != nil {
		return err
	}
	// 3. Send an Execute task.
	if err := stream.Send(&agentpb.ServerMessage{Payload: &agentpb.ServerMessage_Execute{
		Execute: &agentpb.ExecuteTask{JobId: "job-1", Kind: &agentpb.ExecuteTask_Command{
			Command: &agentpb.CommandSpec{Command: "echo from-stream"},
		}},
	}}); err != nil {
		return err
	}
	// 4. Await the TaskResult and assert.
	for {
		msg, err := stream.Recv()
		if err != nil {
			return err
		}
		if r := msg.GetResult(); r != nil {
			if r.JobId != "job-1" || r.Status != "success" || r.Stdout != "from-stream\n" {
				return err
			}
			return nil
		}
	}
}
