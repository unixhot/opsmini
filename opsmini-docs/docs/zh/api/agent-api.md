# Agent 接入（gRPC）

OpsMini 面板进程内置 **Agent 客户端**：它主动向外建立一条 gRPC 双向流长连接，注册自己、上报心跳、接收执行任务、双向传输文件。**不需要对方能 SSH 或能访问本机的入站端口**，适合 NAT、内网、无公网 IP 的主机。

!!! warning "关于旧的 /agent/v1 REST 接口"
    **v1.2.1 起已移除** `/agent/v1` REST 接口与「设置 → API Token」页面。需要从外部系统驱动面板，请使用 **MCP**（`/mcp`，见 [面板 API](browser-api.md)）；需要多机纳管，请使用本文的 gRPC Agent。

## 通信模型

| 方向 | |
|------|------|
| Agent → Server | `Register` · `Heartbeat` · `TaskResult` · `FileChunkAck` |
| Server → Agent | `RegisterAck` · `ExecuteTask` · `PushFileTask` · `Ping` |

- 单端口即可：HTTP/2 多路复用，不需要 SaltStack 那样的双端口
- 断线指数退避重连，周期心跳保活
- Server 端同名主机重连时踢掉旧连接，避免僵尸会话

## 配置

```yaml
agent:
  server_addr: "47.104.172.52:4507"
  server_token: "<token from server>"
  heartbeat_seconds: 30
```

`server_addr` 留空即禁用 Agent 模式，面板仍可正常单机使用。

## 能力边界

| 能力 | 说明 |
|------|------|
| 命令 / 脚本执行 | `ExecuteTask` 的 oneof 区分 command 与 script |
| 文件下发 | `PushFileTask` 服务端流式分片，支持 `resume_offset` 与 md5 校验 |
| 文件拉取 | Agent 侧读取后回传 |
| 指标采集 | 不走 gRPC，仍由 `/metrics` 暴露给 Prometheus 抓取 |

!!! note "Server 端代码位置"
    OpsMini Server 位于 OpsAnt 项目（`github.com/opsany/opsmini-server`），本仓库只包含 Agent 端实现（`internal/agent/client.go`）。
