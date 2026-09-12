# 主配置檔案

OpsMini 使用 YAML 配置檔案，預設路徑 `configs/config.yaml`，可用 `-config` 引數指定。配置檔案不存在時，程式使用內建預設值啟動。

官方隨包提供的示例已精簡為**最小可執行配置**，其餘執行時配置（AI、Agent 令牌、監控認證等）已遷移到面板「設定」頁。

```yaml
# OpsMini Agent 配置文件
server:
  host: "0.0.0.0"        # 监听地址
  port: 8888              # 监听端口
  secret_entry: ""        # 安全入口路径前缀，如 /opsmini_panel（留空不启用）

database:
  path: "opsmini.db"      # SQLite 数据文件路径

jwt:
  access_ttl_seconds: 900     # access token 有效期（15 分钟）
  refresh_ttl_seconds: 604800 # refresh token 有效期（7 天）

log:
  level: error               # 日誌級別：info / warn / error
  path: ""                   # 日誌檔案路徑，留空則只輸出到 stdout（systemd journal）
```

## 配置項說明

### server

| 引數 | 說明 | 預設 |
|------|------|------|
| `host` | 監聽地址，`0.0.0.0` 表示所有網絡卡 | `0.0.0.0` |
| `port` | 監聽埠 | `8888` |
| `secret_entry` | 安全入口路徑字首，如 `/opsmini_panel`，留空不啟用 | 空 |

配置 `secret_entry` 後，面板與所有 API 都掛在字首路徑下（如 `http://<host>:8888/opsmini_panel`），可配合反向代理隱藏真實入口，防埠掃描與暴力探測。

### database

| 引數 | 說明 | 預設 |
|------|------|------|
| `path` | SQLite 資料檔案路徑 | `opsmini.db` |

### jwt

| 引數 | 說明 | 預設 |
|------|------|------|
| `access_ttl_seconds` | access token 有效期（秒） | `900` |
| `refresh_ttl_seconds` | refresh token 有效期（秒） | `604800` |

> JWT 簽名金鑰**不再**在此配置。首次啟動時自動生成 32 位元組隨機金鑰並持久化到資料庫，不落配置檔案、也不在介面展示，無需手動維護。

### log

| 引數 | 說明 | 預設 |
|------|------|------|
| `level` | 日誌級別：`info` / `warn` / `error` | `error` |
| `path` | 日誌檔案路徑，留空則只輸出到 stdout（systemd journal） | 空 |

日誌級別控制輸出詳細程度：`error` 僅輸出錯誤（生產推薦，避免 SQL 查詢日誌刷屏）；`warn` 額外輸出慢查詢與警告；`info` 輸出全部日誌（含 SQL 查詢，適合排查問題）。一鍵安裝預設寫入 `/data/opsmini/opsmini.log`。

## 可選配置段

以下欄位在配置結構中仍受支援，但官方示例已省略（使用內建預設值），可按需顯式宣告。

### agent（命令白名單）

```yaml
agent:
  allowed_commands:        # Agent API 命令白名单前缀
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

| 引數 | 說明 | 預設 |
|------|------|------|
| `allowed_commands` | 允許 `/agent/v1/commands` 執行的命令字首白名單 | 見上 |

> `agent.token` 已廢棄。Agent API 的認證令牌改由面板「設定 → API Token」頁生成與管理，留空即禁用 Agent API。

### metrics（Prometheus 指標）

```yaml
metrics:
  enabled: true     # 是否启用 /metrics 端点
  user: ""          # Basic 认证用户名，空则无认证
  password: ""      # Basic 认证密码
```

| 引數 | 說明 | 預設 |
|------|------|------|
| `enabled` | 是否啟用 `/metrics` 端點 | `true` |
| `user` | HTTP Basic 認證使用者名稱，空則無認證 | 空 |
| `password` | HTTP Basic 認證密碼 | 空 |

> 認證憑據優先順序：面板「設定 → 監控匯出」中的使用者名稱/密碼 **優先於** 這裡的 `user`/`password`。使用者名稱留空即開放訪問。

## 已遷移到面板設定的配置

以下配置項已從 `config.yaml` 遷出，統一在面板「設定」頁管理（儲存於 SQLite，執行時即時生效）：

| 原配置 | 現管理位置 | 說明 |
|--------|-----------|------|
| `jwt.secret` | 自動生成（無需管理） | 首次啟動生成隨機金鑰存庫 |
| `ai.*` | 設定 → AI 大模型接入 | 模型 / API Key / Base URL / 啟用開關 |
| `agent.token` | 設定 → API Token | Agent API 訪問令牌，留空禁用 |
| `metrics.user/password` | 設定 → 監控匯出 | 可執行時覆蓋配置檔案值 |

詳見 [面板設定panel-settings.md。

## 環境變數

| 變數 | 說明 |
|------|------|
| `OPSMINI_AI_KEY` | AI API Key，**優先順序最高**（高於面板設定與配置檔案，避免金鑰落盤） |

## 命令列引數

| 引數 | 說明 |
|------|------|
| `-config <path>` | 指定配置檔案路徑，預設 `configs/config.yaml` |
| `-version` | 列印版本資訊並退出 |
| `-reset-mfa <username>` | 重置指定使用者的 MFA 繫結（丟失驗證碼時使用），完成後退出 |
| `-reset-pass <username>` | 重置指定使用者密碼為隨機強密碼並列印，完成後退出 |

> `-reset-mfa` / `-reset-pass` 直接運算元據庫，不啟動服務，用於賬號丟失恢復。
