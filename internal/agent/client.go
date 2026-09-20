// Package agent implements the OpsMini Agent gRPC client.
//
// The Agent (this process) actively dials an OpsAnt "OpsMini Server" over a
// single gRPC bidirectional stream: it registers itself, sends periodic
// heartbeats, receives Execute / PushFile tasks, executes them, and reports
// results back. It reconnects with exponential backoff on failure.
//
// This is an OPTIONAL capability. It is enabled only when configured
// (agent.server_addr + agent.token) and runs independently of the standalone
// single-host panel, which continues to work exactly as before.
package agent

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/keepalive"

	agentpb "github.com/opsmini/opsmini/internal/agent/pb"
)

// Config holds the Agent connection configuration.
type Config struct {
	ServerAddr  string        // OpsMini Server address (host:port)
	Token       string        // authentication token
	PanelPort   int32         // 面板 Web 端口（上报给 Server，用于 OpsAnt 免认证跳转）
	Heartbeat   time.Duration // heartbeat interval (default 30s)
	RetryMin    time.Duration // min reconnect backoff (default 1s)
	RetryMax    time.Duration // max reconnect backoff (default 30s)
	ExecTimeout time.Duration // default command/script timeout (default 300s)
}

// withDefaults fills zero-valued durations with sane defaults.
func (c Config) withDefaults() Config {
	if c.Heartbeat <= 0 {
		c.Heartbeat = 30 * time.Second
	}
	if c.RetryMin <= 0 {
		c.RetryMin = time.Second
	}
	if c.RetryMax <= 0 {
		c.RetryMax = 30 * time.Second
	}
	if c.ExecTimeout <= 0 {
		c.ExecTimeout = 300 * time.Second
	}
	return c
}

// Client is the Agent gRPC client. Run blocks until ctx is cancelled.
type Client struct {
	cfg       Config
	hostname  string
	version   string
	panelPort int32
	agentID   string
}

// New creates an Agent client. hostname is usually os.Hostname(); version is the build version.
func New(cfg Config, hostname, version string) *Client {
	return &Client{cfg: cfg.withDefaults(), hostname: hostname, version: version, panelPort: cfg.PanelPort}
}

// Run connects and reconnects until ctx is cancelled. Safe to call once per process.
func (c *Client) Run(ctx context.Context) {
	backoff := c.cfg.RetryMin
	for {
		if ctx.Err() != nil {
			return
		}
		err := c.connect(ctx)
		if ctx.Err() != nil {
			return
		}
		log.Printf("agent: disconnected from %s: %v (retry in %s)", c.cfg.ServerAddr, err, backoff)
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > c.cfg.RetryMax {
			backoff = c.cfg.RetryMax
		}
	}
}

// connect performs a single dial + register + receive loop; returns when the stream breaks.
func (c *Client) connect(ctx context.Context) error {
	conn, err := grpc.NewClient(c.cfg.ServerAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithKeepaliveParams(keepalive.ClientParameters{
			Time:                30 * time.Second,
			Timeout:             10 * time.Second,
			PermitWithoutStream: true,
		}),
	)
	if err != nil {
		return err
	}
	defer conn.Close()

	stream, err := agentpb.NewAgentServiceClient(conn).Stream(ctx)
	if err != nil {
		return err
	}

	// Register first.
	if err := stream.Send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Register{
		Register: &agentpb.Register{
			Token:     c.cfg.Token,
			Hostname:  c.hostname,
			Version:   c.version,
			Os:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			PanelPort: c.panelPort,
		},
	}}); err != nil {
		return err
	}

	// Await RegisterAck.
	ackMsg, err := stream.Recv()
	if err != nil {
		return err
	}
	ack := ackMsg.GetRegisterAck()
	if ack == nil {
		return fmt.Errorf("expected RegisterAck, got %T", ackMsg.Payload)
	}
	if !ack.Ok {
		return fmt.Errorf("registration rejected: %s", ack.Message)
	}
	c.agentID = ack.AgentId
	log.Printf("agent: registered with %s as agent_id=%s", c.cfg.ServerAddr, ack.AgentId)

	// Single writer goroutine: all sends (heartbeat / result / file ack) go through sendCh,
	// so only one goroutine ever calls stream.Send (gRPC streams are not safe for concurrent Send).
	sendCh := make(chan *agentpb.AgentMessage, 32)
	sendDone := make(chan struct{})
	go func() {
		defer close(sendDone)
		ticker := time.NewTicker(c.cfg.Heartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := stream.Send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Heartbeat{
					Heartbeat: &agentpb.Heartbeat{AgentId: ack.AgentId, Timestamp: time.Now().Unix()},
				}}); err != nil {
					return
				}
			case m := <-sendCh:
				if err := stream.Send(m); err != nil {
					return
				}
			}
		}
	}()

	// Receive loop: dispatch tasks to worker goroutines; results flow back via sendCh.
	for {
		msg, err := stream.Recv()
		if err != nil {
			<-sendDone
			return err
		}
		switch p := msg.Payload.(type) {
		case *agentpb.ServerMessage_Execute:
			go c.handleExecute(ctx, p.Execute, sendCh)
		case *agentpb.ServerMessage_PushFile:
			go c.handlePushFile(ctx, p.PushFile, sendCh)
		case *agentpb.ServerMessage_PullFile:
			go c.handlePullFile(ctx, p.PullFile, sendCh)
		case *agentpb.ServerMessage_Ping:
			sendCh <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Heartbeat{
				Heartbeat: &agentpb.Heartbeat{AgentId: ack.AgentId, Timestamp: time.Now().Unix()},
			}}
		}
	}
}

// handleExecute runs an Execute task and reports a TaskResult.
func (c *Client) handleExecute(ctx context.Context, t *agentpb.ExecuteTask, sendCh chan<- *agentpb.AgentMessage) {
	result := c.execute(ctx, t)
	sendCh <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Result{Result: result}}
}

// execute performs a command or script task and builds a TaskResult.
func (c *Client) execute(ctx context.Context, t *agentpb.ExecuteTask) *agentpb.TaskResult {
	result := &agentpb.TaskResult{JobId: t.JobId}

	timeout := c.cfg.ExecTimeout
	if t.Timeout > 0 {
		timeout = time.Duration(t.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var cmd *exec.Cmd
	switch kind := t.Kind.(type) {
	case *agentpb.ExecuteTask_Command:
		cmd = exec.CommandContext(ctx, "sh", "-c", kind.Command.Command)
	case *agentpb.ExecuteTask_Script:
		script := kind.Script.Script
		language := kind.Script.Language
		if language == "" {
			language = "shell"
		}
		ext := ".sh"
		if language == "python" {
			ext = ".py"
		}
		tmp, err := os.CreateTemp("", "opsmini-agent-*"+ext)
		if err != nil {
			result.Status = "failed"
			result.Stderr = "create temp file: " + err.Error()
			return result
		}
		tmpPath := tmp.Name()
		defer os.Remove(tmpPath)
		if _, err := tmp.WriteString(script); err != nil {
			tmp.Close()
			result.Status = "failed"
			result.Stderr = "write temp file: " + err.Error()
			return result
		}
		tmp.Close()
		if language == "python" {
			cmd = exec.CommandContext(ctx, "python3", tmpPath)
		} else {
			cmd = exec.CommandContext(ctx, "sh", tmpPath)
		}
	default:
		result.Status = "failed"
		result.Stderr = "execute task has no command or script"
		return result
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	runErr := cmd.Run()
	result.DurationMs = time.Since(start).Milliseconds()

	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		result.Status = "timeout"
		result.ExitCode = -1
	case runErr == nil:
		result.Status = "success"
		result.ExitCode = 0
	default:
		result.Status = "failed"
		if ee, ok := runErr.(*exec.ExitError); ok {
			result.ExitCode = int32(ee.ExitCode())
		} else {
			result.ExitCode = -1
			result.Stderr += runErr.Error()
		}
	}
	return result
}

// handlePushFile receives file chunks for a PushFile task and reports a FileChunkAck.
func (c *Client) handlePushFile(ctx context.Context, t *agentpb.PushFileTask, sendCh chan<- *agentpb.AgentMessage) {
	ack := c.receiveFile(ctx, t)
	sendCh <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_FileAck{FileAck: ack}}
}

// receiveFile writes chunk data to the target path and returns an ack.
// The last chunk triggers an MD5 verification before the ack is reported.
func (c *Client) receiveFile(ctx context.Context, t *agentpb.PushFileTask) *agentpb.FileChunkAck {
	ack := &agentpb.FileChunkAck{JobId: t.JobId}

	if t.Path == "" || !filepath.IsAbs(t.Path) {
		ack.Error = "invalid absolute path: " + t.Path
		return ack
	}

	// 目标路径解析：若 path 是已存在的目录，则把文件放入该目录并保留原文件名；
	// 否则视为文件路径（重命名）。
	targetPath := t.Path
	if st, err := os.Stat(t.Path); err == nil && st.IsDir() {
		if t.Filename == "" {
			ack.Error = "target is a directory but no filename provided"
			return ack
		}
		targetPath = filepath.Join(t.Path, t.Filename)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		ack.Error = "mkdir: " + err.Error()
		return ack
	}

	// Write (append) this chunk at the given offset for resumable transfer.
	f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		ack.Error = "open: " + err.Error()
		return ack
	}
	defer f.Close()
	if _, err := f.WriteAt(t.Chunk, t.Offset); err != nil {
		ack.Error = "write: " + err.Error()
		return ack
	}
	ack.Received = t.Offset + int64(len(t.Chunk))

	// Last chunk: verify size + md5, then apply mode.
	if t.Last {
		if st, err := os.Stat(targetPath); err == nil && t.Size > 0 && st.Size() != t.Size {
			ack.Error = fmt.Sprintf("size mismatch: want %d got %d", t.Size, st.Size())
			return ack
		}
		if t.Md5 != "" {
			data, err := os.ReadFile(targetPath)
			if err != nil {
				ack.Error = "read for md5: " + err.Error()
				return ack
			}
			sum := md5.Sum(data)
			if hex.EncodeToString(sum[:]) != t.Md5 {
				ack.Error = "md5 mismatch"
				return ack
			}
		}
		ack.Md5 = t.Md5
		if t.Mode != "" {
			if m, err := strconv.ParseUint(t.Mode, 8, 32); err == nil {
				_ = os.Chmod(targetPath, os.FileMode(m))
			}
		}
	}
	return ack
}

// chunkSize is the fixed chunk size for file transfer (64 KiB).
const chunkSize = 64 * 1024

// handlePullFile responds to a file-pull request by uploading the file in chunks.
func (c *Client) handlePullFile(ctx context.Context, t *agentpb.PullFileTask, sendCh chan<- *agentpb.AgentMessage) {
	send := func(d *agentpb.FileChunkData) {
		sendCh <- &agentpb.AgentMessage{Payload: &agentpb.AgentMessage_FileData{FileData: d}}
	}
	fail := func(jobID, path, msg string) {
		send(&agentpb.FileChunkData{JobId: jobID, Path: path, Error: msg})
	}

	f, err := os.Open(t.Path)
	if err != nil {
		fail(t.JobId, t.Path, err.Error())
		return
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		fail(t.JobId, t.Path, err.Error())
		return
	}
	total := st.Size()

	sum := md5.New()
	buf := make([]byte, chunkSize)
	var offset int64
	for {
		n, readErr := f.Read(buf)
		if n > 0 {
			_, _ = sum.Write(buf[:n])
			offset += int64(n)
			last := offset >= total // offset reaching total means the final chunk
			d := &agentpb.FileChunkData{
				JobId:  t.JobId,
				Path:   t.Path,
				Offset: offset - int64(n),
				Chunk:  buf[:n],
				Size:   total,
				Last:   last,
			}
			if last {
				d.Md5 = hex.EncodeToString(sum.Sum(nil))
			}
			send(d)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			fail(t.JobId, t.Path, readErr.Error())
			return
		}
	}
}
