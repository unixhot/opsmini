# Agent 接入（gRPC）

OpsMini 面板程序內建 **Agent 用戶端**：它主動向外建立一條 gRPC 雙向流長連線，註冊自己、上報心跳、接收執行任務、雙向傳輸檔案。**不需要對方能 SSH 或能存取本機的入站埠**，適合 NAT、內網、無公網 IP 的主機。

!!! warning "關於舊的 /agent/v1 REST 介面"
    **v1.2.1 起已移除** `/agent/v1` REST 介面與「設定 → API Token」頁面。需要從外部系統驅動面板，請使用 **MCP**（`/mcp`，見 [面板 API](browser-api.md)）；需要多機納管，請使用本文的 gRPC Agent。

## 通訊模型

| 方向 | |
|------|------|
| Agent → Server | `Register` · `Heartbeat` · `TaskResult` · `FileChunkAck` |
| Server → Agent | `RegisterAck` · `ExecuteTask` · `PushFileTask` · `Ping` |

- 單一埠即可：HTTP/2 多工，不需要 SaltStack 那樣的雙埠
- 斷線指數退避重連，週期心跳保活
- Server 端同名主機重連時踢掉舊連線，避免殭屍工作階段

## 設定

```yaml
agent:
  server_addr: "47.104.172.52:4507"
  server_token: "<token from server>"
  heartbeat_seconds: 30
```

`server_addr` 留空即停用 Agent 模式，面板仍可正常單機使用。

## 能力邊界

| 能力 | 說明 |
|------|------|
| 指令 / 腳本執行 | `ExecuteTask` 的 oneof 區分 command 與 script |
| 檔案下發 | `PushFileTask` 伺服器端串流分片，支援 `resume_offset` 與 md5 校验 |
| 檔案拉取 | Agent 端讀取後回傳 |
| 指標採集 | 不走 gRPC，仍由 `/metrics` 暴露給 Prometheus 抓取 |

!!! note "Server 端程式碼位置"
    OpsMini Server 位於 OpsAnt 專案（`github.com/opsany/opsmini-server`），本倉庫只包含 Agent 端實作（`internal/agent/client.go`）。
