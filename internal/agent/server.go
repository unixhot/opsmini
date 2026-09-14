package agent

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"

	agentpb "github.com/opsmini/opsmini/internal/agent/pb"
)

// Server implements the OpsMini Server side of the Agent gRPC protocol.
//
// It accepts Agent connections (each Agent dials in over a single bidirectional
// stream), tracks them by hostname, and lets the caller dispatch Execute tasks
// and collect TaskResults. This is the "control plane" that replaces SaltStack's
// salt-master for the OpsMini execution engine.
type Server struct {
	agentpb.UnimplementedAgentServiceServer

	token  string
	mu     sync.Mutex
	agents map[string]*AgentConn // hostname -> conn
}

// AgentConn is a live connection to one registered Agent.
type AgentConn struct {
	ID       string // hostname
	Version  string
	LastSeen time.Time

	stream  agentpb.AgentService_StreamServer
	sendCh  chan *agentpb.ServerMessage
	results map[string]chan *agentpb.TaskResult // jobID -> result chan
	done    chan struct{}
	mu      sync.Mutex
}

// NewServer creates a Server. token is optional; empty disables registration auth.
func NewServer(token string) *Server {
	return &Server{token: token, agents: make(map[string]*AgentConn)}
}

// Serve runs the gRPC server on lis, blocking until the listener closes.
func (s *Server) Serve(lis net.Listener) error {
	grpcServer := grpc.NewServer()
	agentpb.RegisterAgentServiceServer(grpcServer, s)
	return grpcServer.Serve(lis)
}

// Agent returns the live AgentConn for hostname, or nil if not connected.
func (s *Server) Agent(hostname string) *AgentConn {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.agents[hostname]
}

// Agents returns the hostnames of all currently connected Agents.
func (s *Server) Agents() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.agents))
	for id := range s.agents {
		ids = append(ids, id)
	}
	return ids
}

// Execute sends an ExecuteTask to the given host and blocks until the TaskResult arrives.
func (s *Server) Execute(ctx context.Context, hostname string, task *agentpb.ExecuteTask) (*agentpb.TaskResult, error) {
	ac := s.Agent(hostname)
	if ac == nil {
		return nil, fmt.Errorf("agent %q not connected", hostname)
	}
	return ac.execute(ctx, task)
}

// execute dispatches a task over the agent's stream and waits for its result.
func (ac *AgentConn) execute(ctx context.Context, task *agentpb.ExecuteTask) (*agentpb.TaskResult, error) {
	resultCh := make(chan *agentpb.TaskResult, 1)
	ac.mu.Lock()
	ac.results[task.JobId] = resultCh
	ac.mu.Unlock()
	defer func() {
		ac.mu.Lock()
		delete(ac.results, task.JobId)
		ac.mu.Unlock()
	}()

	select {
	case ac.sendCh <- &agentpb.ServerMessage{Payload: &agentpb.ServerMessage_Execute{Execute: task}}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ac.done:
		return nil, fmt.Errorf("agent %q disconnected", ac.ID)
	}

	select {
	case r := <-resultCh:
		return r, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ac.done:
		return nil, fmt.Errorf("agent %q disconnected", ac.ID)
	}
}

// Stream implements the bidirectional stream: register, then relay messages.
func (s *Server) Stream(stream agentpb.AgentService_StreamServer) error {
	// 1. Await the Register message.
	msg, err := stream.Recv()
	if err != nil {
		return err
	}
	reg := msg.GetRegister()
	if reg == nil {
		return fmt.Errorf("first message must be Register")
	}
	if s.token != "" && reg.Token != s.token {
		_ = stream.Send(&agentpb.ServerMessage{Payload: &agentpb.ServerMessage_RegisterAck{
			RegisterAck: &agentpb.RegisterAck{Ok: false, Message: "invalid token"},
		}})
		return fmt.Errorf("invalid token from %q", reg.Hostname)
	}

	ac := &AgentConn{
		ID:       reg.Hostname,
		Version:  reg.Version,
		LastSeen: time.Now(),
		stream:   stream,
		sendCh:   make(chan *agentpb.ServerMessage, 32),
		results:  make(map[string]chan *agentpb.TaskResult),
		done:     make(chan struct{}),
	}

	s.mu.Lock()
	old := s.agents[ac.ID]
	s.agents[ac.ID] = ac
	s.mu.Unlock()
	if old != nil {
		close(old.done) // kick the previous connection for the same host
	}
	defer func() {
		s.mu.Lock()
		if s.agents[ac.ID] == ac {
			delete(s.agents, ac.ID)
		}
		s.mu.Unlock()
		close(ac.done)
	}()

	// 2. Send RegisterAck.
	if err := stream.Send(&agentpb.ServerMessage{Payload: &agentpb.ServerMessage_RegisterAck{
		RegisterAck: &agentpb.RegisterAck{Ok: true, AgentId: ac.ID},
	}}); err != nil {
		return err
	}
	log.Printf("agent server: agent %q registered (ver=%s)", ac.ID, ac.Version)

	// 3. Single sender goroutine.
	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		for {
			select {
			case <-ac.done:
				return
			case m := <-ac.sendCh:
				if err := stream.Send(m); err != nil {
					return
				}
			}
		}
	}()

	// 4. Receive loop.
	for {
		msg, err := stream.Recv()
		if err != nil {
			<-sendDone
			return err
		}
		switch p := msg.Payload.(type) {
		case *agentpb.AgentMessage_Heartbeat:
			ac.LastSeen = time.Now()
		case *agentpb.AgentMessage_Result:
			ac.mu.Lock()
			ch := ac.results[p.Result.JobId]
			ac.mu.Unlock()
			if ch != nil {
				ch <- p.Result
			}
		case *agentpb.AgentMessage_FileAck:
			// File chunk acknowledgement; resumable transfer is handled by the caller.
		}
	}
}
