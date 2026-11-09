package agent

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"

	agentpb "github.com/opsmini/opsmini/internal/agent/pb"
)

// A fake Server is enough to prove the whole path: register → receive task →
// execute → report. That is exactly the part which cannot be checked by unit
// tests of either side alone, and the part that breaks first when the protocol
// changes.

type fakeServer struct {
	agentpb.UnimplementedAgentServiceServer
	pluginTasks []*agentpb.PluginTask
	// rawMsgs are sent verbatim so a test can craft a message the generated
	// types no longer describe — e.g. a field removed from the schema.
	rawMsgs     []*agentpb.ServerMessage
	plugResults chan *agentpb.PluginResult
	heartbeats  chan *agentpb.Heartbeat
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
	for _, m := range f.rawMsgs {
		if err := stream.Send(m); err != nil {
			return err
		}
	}
	for _, p := range f.pluginTasks {
		if err := stream.Send(&agentpb.ServerMessage{Payload: &agentpb.ServerMessage_PluginTask{PluginTask: p}}); err != nil {
			return err
		}
	}
	if f.heartbeats != nil {
		// Liveness probe: if the client survived whatever we just sent, it
		// answers this with a heartbeat.
		if err := stream.Send(&agentpb.ServerMessage{Payload: &agentpb.ServerMessage_Ping{Ping: &agentpb.Ping{}}}); err != nil {
			return err
		}
	}
	for {
		in, err := stream.Recv()
		if err != nil {
			return nil
		}
		switch p := in.Payload.(type) {
		case *agentpb.AgentMessage_PluginResult:
			select {
			case f.plugResults <- p.PluginResult:
			default:
			}
		case *agentpb.AgentMessage_Heartbeat:
			select {
			case f.heartbeats <- p.Heartbeat:
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

// TestRemovedLogTaskIsIgnored pins the behaviour agreed for v1.4.2: the built-in
// log pipeline is gone, but an older OpsAnt may still send a LogTask (field 6,
// which is now reserved). protobuf stores it as an unknown field and leaves
// Payload nil, so no handler ever runs. What matters is that the stream survives
// — a panic or a dropped connection here would take down every other task
// channel with it.
func TestRemovedLogTaskIsIgnored(t *testing.T) {
	var raw []byte
	raw = protowire.AppendTag(raw, 6, protowire.BytesType)
	raw = protowire.AppendBytes(raw, []byte("legacy-log-task-payload"))

	legacy := &agentpb.ServerMessage{}
	legacy.ProtoReflect().SetUnknown(protoreflect.RawFields(raw))

	f := &fakeServer{
		rawMsgs:     []*agentpb.ServerMessage{legacy},
		plugResults: make(chan *agentpb.PluginResult, 4),
		heartbeats:  make(chan *agentpb.Heartbeat, 4),
		registered:  make(chan *agentpb.Register, 1),
	}
	addr, stop := startFake(t, f)
	defer stop()

	cl := New(Config{ServerAddr: addr, Token: "t", Heartbeat: time.Second}, "test-host", "test", "opsmini-testhost01")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go cl.Run(ctx)

	select {
	case <-f.heartbeats:
		// Ping answered: the stream is still alive after the removed message.
	case <-time.After(10 * time.Second):
		t.Fatal("stream stopped responding after a legacy LogTask")
	}
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
