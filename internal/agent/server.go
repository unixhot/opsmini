package agent

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/keepalive"

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

	token            string
	heartbeatTimeout time.Duration // agents with LastSeen older than this are considered offline
	mu               sync.Mutex
	agents           map[string]*AgentConn // hostname -> conn
}

// AgentConn is a live connection to one registered Agent.
type AgentConn struct {
	ID          string // hostname
	Version     string
	ConnectedAt time.Time
	LastSeen    time.Time

	stream  agentpb.AgentService_StreamServer
	sendCh  chan *agentpb.ServerMessage
	results map[string]chan *agentpb.TaskResult   // jobID -> result chan (execute)
	files   map[string]chan *agentpb.FileChunkData // jobID -> chunk chan (pull)
	done    chan struct{}
	mu      sync.Mutex
}

// NewServer creates a Server. token is optional; empty disables registration auth.
// The default heartbeat timeout (used for online/offline status) is 90s.
func NewServer(token string) *Server {
	return &Server{
		token:            token,
		heartbeatTimeout: 90 * time.Second,
		agents:           make(map[string]*AgentConn),
	}
}

// SetHeartbeatTimeout overrides the online/offline threshold (must be set before serving).
func (s *Server) SetHeartbeatTimeout(d time.Duration) {
	if d > 0 {
		s.heartbeatTimeout = d
	}
}

// Serve runs the gRPC server on lis, blocking until the listener closes.
func (s *Server) Serve(lis net.Listener) error {
	grpcServer := grpc.NewServer(
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle:     2 * time.Minute,
			MaxConnectionAgeGrace: 10 * time.Second,
			Time:                  2 * time.Minute,
			Timeout:               20 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             30 * time.Second,
			PermitWithoutStream: true,
		}),
	)
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

// AgentStatus is a snapshot of one Agent's connection state.
type AgentStatus struct {
	Hostname    string    `json:"hostname"`
	Version     string    `json:"version"`
	ConnectedAt time.Time `json:"connected_at"`
	LastSeen    time.Time `json:"last_seen"`
	Online      bool      `json:"online"`
}

// AgentStatuses returns connection status for all currently tracked Agents.
func (s *Server) AgentStatuses() []AgentStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now()
	out := make([]AgentStatus, 0, len(s.agents))
	for _, ac := range s.agents {
		out = append(out, AgentStatus{
			Hostname:    ac.ID,
			Version:     ac.Version,
			ConnectedAt: ac.ConnectedAt,
			LastSeen:    ac.LastSeen,
			Online:      now.Sub(ac.LastSeen) <= s.heartbeatTimeout,
		})
	}
	return out
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

// PullFile pulls a file from the given host's Agent, returning its full contents.
// The data is verified against the size and md5 reported by the Agent.
func (s *Server) PullFile(ctx context.Context, hostname, path string) ([]byte, error) {
	ac := s.Agent(hostname)
	if ac == nil {
		return nil, fmt.Errorf("agent %q not connected", hostname)
	}
	return ac.pullFile(ctx, path)
}

// pullFile dispatches a PullFileTask and assembles the returned chunks.
func (ac *AgentConn) pullFile(ctx context.Context, path string) ([]byte, error) {
	jobID := fmt.Sprintf("pull-%d", time.Now().UnixNano())
	chunkCh := make(chan *agentpb.FileChunkData, 32)
	ac.mu.Lock()
	ac.files[jobID] = chunkCh
	ac.mu.Unlock()
	defer func() {
		ac.mu.Lock()
		delete(ac.files, jobID)
		ac.mu.Unlock()
	}()

	task := &agentpb.PullFileTask{JobId: jobID, Path: path}
	select {
	case ac.sendCh <- &agentpb.ServerMessage{Payload: &agentpb.ServerMessage_PullFile{PullFile: task}}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-ac.done:
		return nil, fmt.Errorf("agent %q disconnected", ac.ID)
	}

	var buf bytes.Buffer
	var expectedMD5 string
	var total int64
	for {
		select {
		case d := <-chunkCh:
			if d.Error != "" {
				return nil, fmt.Errorf("pull %s: %s", path, d.Error)
			}
			_, _ = buf.Write(d.Chunk)
			if d.Last {
				total = d.Size
				expectedMD5 = d.Md5
				goto done
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ac.done:
			return nil, fmt.Errorf("agent %q disconnected", ac.ID)
		}
	}

done:
	data := buf.Bytes()
	if total > 0 && int64(len(data)) != total {
		return nil, fmt.Errorf("pull %s: size mismatch want %d got %d", path, total, len(data))
	}
	if expectedMD5 != "" {
		sum := md5.Sum(data)
		if hex.EncodeToString(sum[:]) != expectedMD5 {
			return nil, fmt.Errorf("pull %s: md5 mismatch", path)
		}
	}
	return data, nil
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

	now := time.Now()
	ac := &AgentConn{
		ID:          reg.Hostname,
		Version:     reg.Version,
		ConnectedAt: now,
		LastSeen:    now,
		stream:      stream,
		sendCh:      make(chan *agentpb.ServerMessage, 32),
		results:     make(map[string]chan *agentpb.TaskResult),
		files:       make(map[string]chan *agentpb.FileChunkData),
		done:        make(chan struct{}),
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
			// File push (Server->Agent) chunk acknowledgement; resumable transfer is handled by the caller.
		case *agentpb.AgentMessage_FileData:
			ac.mu.Lock()
			ch := ac.files[p.FileData.JobId]
			ac.mu.Unlock()
			if ch != nil {
				ch <- p.FileData
			}
		}
	}
}
