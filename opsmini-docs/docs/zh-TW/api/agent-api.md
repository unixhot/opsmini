# Agent API（/agent/v1）

Agent API 是面向**機器 / 第三方系統**的標準 REST 介面，供監控平臺、自動化指令碼、編排工具整合呼叫。

## 與面板 API 的區別

| 維度 | 面板 API `/api/v1` | Agent API `/agent/v1` |
|------|-------------------|----------------------|
| 使用者 | 瀏覽器（人） | 外部系統（機器） |
| 認證 | 使用者會話 JWT + RBAC | Agent Token（Bearer） |
| 範圍 | 全部 UI 功能 + 終端 WS | 資源查詢 + 命令執行（無 UI / 終端） |
| 設計 | 面向互動 | 面向自動化（冪等、可重試） |

## 認證

1. 在面板「設定 → API Token」頁生成訪問令牌（令牌持久化在資料庫，不再寫在 `config.yaml`）
2. 請求攜帶 `Authorization: Bearer <token>`
3. 中介軟體以恆定時間比較校驗 token；令牌留空即禁用 Agent API

## 端點一覽

| 方法 | 路徑 | 說明 |
|------|------|------|
| GET | `/agent/v1/health` | 健康檢查（探活） |
| GET | `/agent/v1/version` | 版本資訊 |
| GET | `/agent/v1/status` | 狀態摘要：cpu / mem / 磁碟 / 線上服務 |
| GET | `/agent/v1/system/info` | 主機資訊（hostname / os / 核心） |
| GET | `/agent/v1/system/processes` | 程序列表 |
| GET | `/agent/v1/system/ports` | 埠監聽 |
| GET | `/agent/v1/system/disks` | 磁碟 / 掛載點 |
| GET | `/agent/v1/websites` | 網站列表 |
| GET | `/agent/v1/databases` | 資料庫列表 |
| GET | `/agent/v1/cron-jobs` | 計劃任務 |
| GET | `/agent/v1/containers` | 容器列表 |
| POST | `/agent/v1/commands` | 執行命令（白名單） |
| POST | `/agent/v1/script/run` | 執行指令碼 |
| POST | `/agent/v1/file/upload` | 上傳檔案 |

## 命令執行與白名單

`/commands` 僅允許執行顯式授權的命令字首，全部記審計。白名單在 `config.yaml` 的 `agent.allowed_commands` 中維護：

```yaml
agent:
  allowed_commands:
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

## 呼叫示例

```bash
curl -H "Authorization: Bearer <token>" \
     https://<host>:8888/agent/v1/status
```

## 安全建議

- 使用 HTTPS 傳輸
- 高安全場景可加 IP 白名單限制來源
- 命令白名單最小化，僅授權必要命令
- 令牌洩露後立即在面板「設定 → API Token」重新生成
