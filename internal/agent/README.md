# internal/agent

The OpsMini **Agent** gRPC client. This package connects the panel to an OpsAnt
"OpsMini Server" and lets the panel be managed centrally without SSH.

> Not to be confused with [`internal/aiagent`](../aiagent/README.md), which is
> the LLM agent (tool calling, guard rails, MCP).

## Model

The Agent **dials out** to the Server over a single bidirectional gRPC stream
(similar to how a salt-minion connects to a salt-master), so no inbound port has
to be opened on the managed host. It registers itself, sends periodic
heartbeats, receives tasks, executes them and reports the results. On failure it
reconnects with exponential backoff.

```
OpsMini panel (this package)  ──gRPC stream──▶  OpsMini Server (in OpsAnt)
        register / heartbeat                        :4507 data plane
        ◀── ExecuteTask / PushFileTask / PullFileTask
        ──▶ TaskResult / FileChunkAck / FileChunkData      :4508 control plane
```

## Files

| File | Purpose |
|------|---------|
| `client.go` | Connection, registration, keepalive, single-writer send loop, task dispatch |
| `manager.go` | Lifecycle manager: start/stop/reconfigure the Agent at runtime from the panel |
| `pb/` | Generated protobuf/gRPC code from `proto/agent/v1/agent.proto` |
| `client_test.go` | Client unit tests |

## Capabilities

| Capability | Direction | Notes |
|-----------|-----------|-------|
| Execute command / script | Server → Agent | `sh -c` for commands, temp file for scripts, per-task timeout |
| Push a file | Server → Agent | Chunked (64 KiB), `offset` for resume, size + MD5 verification, optional mode |
| Pull a file | Agent → Server | Chunked upload with a trailing MD5 |
| Heartbeat | Agent → Server | Interval from `agent.heartbeat_seconds` (default 30s) |

## Configuration

```yaml
agent:
  server_addr: "47.104.172.52:4507"   # empty disables the Agent (standalone panel unaffected)
  server_token: "..."                 # registration token
  heartbeat_seconds: 30
```

The panel settings page overrides `config.yaml` at runtime through
`Manager.Start` / `Manager.Stop`, so the link can be reconfigured without a
restart.

## Regenerating the protobuf code

The contract lives in `proto/agent/v1/agent.proto`. From the repository root:

```bash
protoc --go_out=. --go_opt=module=github.com/opsmini/opsmini \
       --go-grpc_out=. --go-grpc_opt=module=github.com/opsmini/opsmini \
       proto/agent/v1/agent.proto
```

The OpsAnt side keeps its own copy of the same contract with its own
`go_package`; gRPC interoperability depends only on the wire format.
