# Agent access (gRPC)

The OpsMini process embeds an **Agent client**: it dials out and holds one long-lived gRPC bidirectional stream to register itself, send heartbeats, receive tasks and transfer files both ways. **It needs no inbound port and no SSH** — which makes it usable behind NAT, in private networks and on hosts without a public IP.

!!! warning "About the old /agent/v1 REST API"
    The `/agent/v1` REST API and the "Settings → API Token" page were **removed in v1.2.1**. To drive the panel from an external system use **MCP** (`/mcp`, see [Panel API](browser-api.md)); for fleet management use the gRPC agent described here.

## Communication model

| Direction | |
|------|------|
| Agent → Server | `Register` · `Heartbeat` · `TaskResult` · `FileChunkAck` |
| Server → Agent | `RegisterAck` · `ExecuteTask` · `PushFileTask` · `Ping` |

- One port is enough: HTTP/2 multiplexing, no SaltStack-style dual port
- Exponential backoff reconnect with periodic heartbeats
- A reconnect from the same host name kicks the stale connection

## Configuration

```yaml
agent:
  server_addr: "47.104.172.52:4507"
  server_token: "<token from server>"
  heartbeat_seconds: 30
```

Leaving `server_addr` empty disables agent mode; the panel keeps working standalone.

## Capabilities

| Capability | Notes |
|------|------|
| Command / script execution | `ExecuteTask` uses a oneof to distinguish command from script |
| File push | `PushFileTask` streams chunks with `resume_offset` and md5 verification |
| File pull | The agent reads the file and streams it back |
| Metrics | Not over gRPC — still exposed on `/metrics` for Prometheus |

!!! note "Where the server lives"
    OpsMini Server lives in the OpsAnt project (`github.com/opsany/opsmini-server`); this repository only contains the agent side (`internal/agent/client.go`).
