package agent

import (
	"context"
	"errors"
	"net"
	"runtime"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	agentpb "github.com/opsmini/opsmini/internal/agent/pb"
)

// Probe stages, returned so the panel can localize the failure reason instead
// of showing a raw Go error string.
const (
	ProbeStageFormat   = "format"   // address is not host:port
	ProbeStageTCP      = "tcp"      // TCP dial failed (unreachable / port closed)
	ProbeStageRegister = "register" // gRPC stream or registration failed (bad token, wrong server)
	ProbeStageDone     = "done"     // registration accepted
)

// ProbeResult is the outcome of a single connectivity test.
type ProbeResult struct {
	OK            bool   `json:"ok"`
	Stage         string `json:"stage"`
	Message       string `json:"message"`
	LatencyMS     int64  `json:"latency_ms"`
	ServerAgentID string `json:"server_agent_id,omitempty"`
	// Detail keeps the raw error for troubleshooting; the panel shows it in
	// small type under the localized headline.
	Detail string `json:"detail,omitempty"`
}

// ProbeOnce performs one real handshake against an OpsAnt OpsMini Server:
// TCP dial + gRPC stream + Register, then waits for RegisterAck. It verifies
// the address AND the token, which a bare TCP check cannot do.
//
// The caller is responsible for making sure this does not race with the live
// connection: the Server keys agents by hostname and kicks the previous
// connection when the same hostname registers again.
func ProbeOnce(ctx context.Context, cfg Config, hostname, version, instanceID string) ProbeResult {
	start := time.Now()
	fail := func(stage, msg string, detail error) ProbeResult {
		r := ProbeResult{Stage: stage, Message: msg, LatencyMS: time.Since(start).Milliseconds()}
		if detail != nil {
			r.Detail = detail.Error()
		}
		return r
	}

	addr := strings.TrimSpace(cfg.ServerAddr)
	host, port, err := net.SplitHostPort(addr)
	if err != nil || strings.TrimSpace(host) == "" || strings.TrimSpace(port) == "" {
		return fail(ProbeStageFormat, "地址格式应为 host:port，例如 10.0.0.1:4507", nil)
	}
	if n, convErr := strconv.Atoi(port); convErr != nil || n < 1 || n > 65535 {
		return fail(ProbeStageFormat, "端口号无效："+port, nil)
	}

	// 1) TCP dial first: it separates "port not reachable" from "token wrong",
	// which is the single most useful distinction when a user is debugging.
	dialer := &net.Dialer{Timeout: 4 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return fail(ProbeStageTCP, "无法连接 "+addr+"："+dialHint(err), err)
	}
	_ = conn.Close()

	// 2) gRPC handshake + registration.
	cctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	gconn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fail(ProbeStageRegister, "创建 gRPC 连接失败", err)
	}
	// Terminate the stream explicitly before dropping the connection. Relying on
	// keepalive leaves the Server holding the entry, which shows up as a ghost
	// agent in the OpsAnt host list.
	defer func() {
		cancel()
		_ = gconn.Close()
	}()

	// One retry: when the panel already holds a live agent, stopping it and
	// re-registering can lose a race with the Server tearing down the previous
	// stream (EOF / Unavailable). That is a timing artifact, not a bad config,
	// so retrying avoids reporting a false negative.
	var ack *agentpb.RegisterAck
	var recvErr error
	for attempt := 1; attempt <= 2; attempt++ {
		ack, recvErr = registerOnce(cctx, gconn, cfg, hostname, version, instanceID)
		if recvErr == nil || !transientStreamErr(recvErr) {
			break
		}
		select {
		case <-cctx.Done():
		case <-time.After(350 * time.Millisecond):
		}
	}
	if recvErr != nil {
		if errors.Is(recvErr, context.DeadlineExceeded) {
			return fail(ProbeStageRegister, "已连通但 "+addr+" 未响应注册请求（对端可能不是 OpsMini Server，或端口填错）", recvErr)
		}
		return fail(ProbeStageRegister, "等待注册回执失败："+dialHint(recvErr), recvErr)
	}
	if !ack.GetOk() {
		return fail(ProbeStageRegister, "注册被拒绝："+ack.GetMessage()+"（通常是 Agent Token 不正确）", nil)
	}

	return ProbeResult{
		OK:            true,
		Stage:         ProbeStageDone,
		Message:       "连接成功",
		LatencyMS:     time.Since(start).Milliseconds(),
		ServerAgentID: ack.GetAgentId(),
	}
}

// registerOnce opens one stream, sends Register and waits for RegisterAck.
func registerOnce(ctx context.Context, conn *grpc.ClientConn, cfg Config, hostname, version, instanceID string) (*agentpb.RegisterAck, error) {
	stream, err := agentpb.NewAgentServiceClient(conn).Stream(ctx)
	if err != nil {
		return nil, err
	}
	if err := stream.Send(&agentpb.AgentMessage{Payload: &agentpb.AgentMessage_Register{
		Register: &agentpb.Register{
			Token:     cfg.Token,
			Hostname:  hostname,
			Version:   version,
			Os:        runtime.GOOS,
			Arch:      runtime.GOARCH,
			PanelPort: cfg.PanelPort,
			AgentId:   instanceID,
		},
	}}); err != nil {
		return nil, err
	}
	msg, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	ack := msg.GetRegisterAck()
	if ack == nil {
		return nil, errors.New("对端返回了非注册响应，可能不是 OpsMini Server")
	}
	return ack, nil
}

// transientStreamErr reports whether a stream error is the kind that a retry
// can plausibly fix (connection closing under us) rather than a definitive
// answer from the Server.
func transientStreamErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "EOF") ||
		strings.Contains(msg, "transport is closing") ||
		strings.Contains(msg, "code = Unavailable")
}

// dialHint turns the common syscall errors into something an operator can act
// on, while keeping the original text available in Detail.
func dialHint(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "connection refused"):
		return "目标端口没有服务在监听（connection refused）"
	case strings.Contains(msg, "i/o timeout"), strings.Contains(msg, "deadline exceeded"):
		return "连接超时，网络不通或被防火墙/安全组拦截"
	case strings.Contains(msg, "no route to host"):
		return "主机不可达（no route to host）"
	case strings.Contains(msg, "no such host"):
		return "域名解析失败（no such host）"
	case strings.Contains(msg, "connect: network is unreachable"):
		return "网络不可达，请检查本机网络或路由"
	}
	return msg
}
