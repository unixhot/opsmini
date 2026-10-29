package agent

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc"

	agentpb "github.com/opsmini/opsmini/internal/agent/pb"
	"github.com/opsmini/opsmini/internal/logpipe"
)

// A fake Server is enough to prove the whole path: register → receive task →
// execute → report. That is exactly the part which cannot be checked by unit
// tests of either side alone, and the part that breaks first when the protocol
// changes.

type fakeServer struct {
	agentpb.UnimplementedAgentServiceServer
	logTasks    []*agentpb.LogTask
	pluginTasks []*agentpb.PluginTask
	logResults  chan *agentpb.LogTaskResult
	plugResults chan *agentpb.PluginResult
	registered  chan *agentpb.Register
}

func (f *fakeServer) Stream(stream agentpb.AgentService_StreamServer) error {
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	if reg, ok := msg.Payload.(*agentpb.AgentMessage_Register); ok {
		f.registered <- reg.Register
	}
	if err := stream.Send(&agentpb.ServerMessage{Payload: &agentpb.ServerMessage_RegisterAck{
		RegisterAck: &agentpb.RegisterAck{Ok: true, AgentId: "test-agent"},
	}}); err != nil {
		return err
	}
	// Give the client a moment to settle after the ack before pushing work.
	time.Sleep(100 * time.Millisecond)
	for _, t := range f.logTasks {
		if err := stream.Send(&agentpb.ServerMessage{Payload: &agentpb.ServerMessage_LogTask{LogTask: t}}); err != nil {
			return err
		}
	}
	for _, p := range f.pluginTasks {
		if err := stream.Send(&agentpb.ServerMessage{Payload: &agentpb.ServerMessage_PluginTask{PluginTask: p}}); err != nil {
			return err
		}
	}
	for {
		in, err := stream.Recv()
		if err != nil {
			return nil
		}
		switch p := in.Payload.(type) {
		case *agentpb.AgentMessage_LogResult:
			select {
			case f.logResults <- p.LogResult:
			default:
			}
		case *agentpb.AgentMessage_PluginResult:
			select {
			case f.plugResults <- p.PluginResult:
			default:
			}
		}
	}
}

// logAdapter adapts the pipeline to the interface the client expects (the router
// has its own copy; the agent package must not import the router).
type logAdapter struct{ mgr *logpipe.Manager }

func (a logAdapter) ApplyTask(f string, c []byte, s string, strict bool) (string, []string, error) {
	return a.mgr.ApplyTask(f, c, s, strict)
}
func (a logAdapter) RemoveTask(f string) error { return a.mgr.RemoveTask(f) }
func (a logAdapter) ReloadNow()                { a.mgr.ReloadNow() }
func (a logAdapter) TaskByFile(f string) (LogState, bool) {
	st, ok := a.mgr.TaskByFile(f)
	if !ok {
		return LogState{}, false
	}
	return LogState{TaskID: st.TaskID, State: st.State, EventsSent: st.EventsSent}, true
}

func startFake(t *testing.T, f *fakeServer) (string, func()) {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	agentpb.RegisterAgentServiceServer(srv, f)
	go srv.Serve(lis)
	return lis.Addr().String(), srv.Stop
}

func TestLogTaskReachesPipelineAndReportsBack(t *testing.T) {
	dir := t.TempDir()
	tasks := filepath.Join(dir, "tasks.d")
	if err := os.MkdirAll(tasks, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cfg := logpipe.DefaultMainConfig(dir)
	cfg.TasksDir = tasks
	mgr := logpipe.NewManager(cfg, nil, "test")

	content := []byte("- type: filestream\n  id: task-900\n  paths: [\"" + filepath.Join(dir, "app.log") + "\"]\n")
	f := &fakeServer{
		logTasks: []*agentpb.LogTask{{
			JobId: "job-1", Action: "apply", TaskId: "900",
			Filename: "task-900.yml", Content: content,
		}},
		logResults:  make(chan *agentpb.LogTaskResult, 4),
		plugResults: make(chan *agentpb.PluginResult, 4),
		registered:  make(chan *agentpb.Register, 1),
	}
	addr, stop := startFake(t, f)
	defer stop()

	cl := New(Config{ServerAddr: addr, Token: "t", Heartbeat: time.Second}, "test-host", "test")
	cl.SetControls(logAdapter{mgr: mgr}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cl.Run(ctx)

	select {
	case res := <-f.logResults:
		if res.Status != "applied" {
			t.Fatalf("status=%s error=%s", res.Status, res.Error)
		}
		if res.TaskId != "900" {
			t.Fatalf("task_id=%s", res.TaskId)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for log task result")
	}

	// The file must exist on disk, not just be acknowledged over the wire.
	if _, err := os.Stat(filepath.Join(tasks, "task-900.yml")); err != nil {
		t.Fatalf("task file not written: %v", err)
	}
}

func TestLogTaskWithoutPipelineFailsLoudly(t *testing.T) {
	f := &fakeServer{
		logTasks: []*agentpb.LogTask{{
			JobId: "job-2", Action: "apply", TaskId: "901", Filename: "x.yml", Content: []byte("x"),
		}},
		logResults:  make(chan *agentpb.LogTaskResult, 4),
		plugResults: make(chan *agentpb.PluginResult, 4),
		registered:  make(chan *agentpb.Register, 1),
	}
	addr, stop := startFake(t, f)
	defer stop()

	cl := New(Config{ServerAddr: addr, Token: "t", Heartbeat: time.Second}, "test-host", "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cl.Run(ctx)

	select {
	case res := <-f.logResults:
		// An unwired subsystem must report a failure, never a silent success.
		if res.Status != "failed" || res.Error == "" {
			t.Fatalf("expected explicit failure, got status=%s error=%q", res.Status, res.Error)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for log task result")
	}
}

func TestPluginTaskWithoutManagerFailsLoudly(t *testing.T) {
	f := &fakeServer{
		pluginTasks: []*agentpb.PluginTask{{
			JobId: "job-3", Action: "install", PluginId: "demo",
			SourceUrl: "http://127.0.0.1:1/none.tar.gz",
		}},
		logResults:  make(chan *agentpb.LogTaskResult, 4),
		plugResults: make(chan *agentpb.PluginResult, 4),
		registered:  make(chan *agentpb.Register, 1),
	}
	addr, stop := startFake(t, f)
	defer stop()

	cl := New(Config{ServerAddr: addr, Token: "t", Heartbeat: time.Second}, "test-host", "test")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cl.Run(ctx)

	select {
	case res := <-f.plugResults:
		if res.Status != "failed" || res.Error == "" {
			t.Fatalf("expected explicit failure, got status=%s error=%q", res.Status, res.Error)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for plugin task result")
	}
}
