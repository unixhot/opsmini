package agent

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"

	agentpb "github.com/opsmini/opsmini/internal/agent/pb"
)

// A fake Server is enough to prove the whole path: register → receive task →
// execute → report. That is exactly the part which cannot be checked by unit
// tests of either side alone, and the part that breaks first when the protocol
// changes.

type fakeServer struct {
	agentpb.UnimplementedAgentServiceServer
	pluginTasks []*agentpb.PluginTask
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
		if p, ok := in.Payload.(*agentpb.AgentMessage_PluginResult); ok {
			select {
			case f.plugResults <- p.PluginResult:
			default:
			}
		}
	}
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

func TestPluginTaskWithoutManagerFailsLoudly(t *testing.T) {
	f := &fakeServer{
		pluginTasks: []*agentpb.PluginTask{{
			JobId: "job-3", Action: "install", PluginId: "demo",
			SourceUrl: "http://127.0.0.1:1/none.tar.gz",
		}},
		plugResults: make(chan *agentpb.PluginResult, 4),
		registered:  make(chan *agentpb.Register, 1),
	}
	addr, stop := startFake(t, f)
	defer stop()

	cl := New(Config{ServerAddr: addr, Token: "t", Heartbeat: time.Second}, "test-host", "test", "opsmini-testhost01")
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
