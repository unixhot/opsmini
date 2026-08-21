<div align="center">

# OpsMini Backend Architecture

**後端架構設計**

[English](backend-architecture.md) · [简体中文](backend-architecture.zh-CN.md) · [繁體中文](backend-architecture.zh-TW.md) · [日本語](backend-architecture.ja.md) · [한국어](backend-architecture.ko.md) · [ไทย](backend-architecture.th.md) · [Deutsch](backend-architecture.de.md)

</div>

---

> 版本：v1.0  
> 依據：`web/index.html` UI 原型（登入/使用者/儀表板/監控/應用/容器/系統/檔案/終端機/排程任務/日誌/AI 助手/面板設定/7 語言 i18n）

---

## 1. 概述

OpsMini 是一款**輕量級單機維運面板**（對標寶塔 / 1Panel），兩大差異化：

1. **內建 AI 大模型**做系統管理（自然語言診斷 / 執行 / 日誌分析）；
2. **對外暴露標準 REST API**，可被外部系統（監控平台、自動化腳本、第三方編排工具）整合呼叫。

> v1.0 範圍說明：本版本**不包含 Proxy 中心節點**。每台主機上的 OpsMini 面板獨立運作，
> 透過標準 REST API 對外提供能力；多機統一管理（Proxy）留待後續版本評估。

### 1.1 設計目標

| 目標 | 說明 |
|------|------|
| 輕量 | 單一二進位部署，記憶體佔用低，適合 1C1G 小型主機 |
| 單機優先 | 核心場景是單台伺服器，不引入分散式複雜度 |
| 可整合 | 透過標準 REST API 暴露能力，供外部系統整合呼叫 |
| 安全 | RBAC、2FA、安全入口、最小權限、金鑰加密儲存 |
| 可維護 | 模組化分層，Controller → Service → Repository 清晰 |

### 1.2 技術堆疊選型

| 層 | 選型 | 備選 | 理由 |
|----|------|------|------|
| 語言 | Go 1.22+ | — | 單一二進位、交叉編譯、並發好、生態成熟（1Panel 同棧） |
| Web 框架 | **Gin** | Echo / chi | 生態最大、中介軟體豐富、1Panel 同款 |
| 資料庫 | **SQLite** | — | 嵌入式、零維運，貼合單機場景 |
| SQLite 驅動 | **modernc.org/sqlite** | mattn/go-sqlite3 | 純 Go 無 CGO，交叉編譯省心 |
| ORM | **GORM** | sqlx | 開發效率高；複雜查詢可回退原生 SQL |
| 即時通訊 | **gorilla/websocket** | — | 終端機、日誌 tail、指標推送 |
| 任務排程 | **robfig/cron** | — | 面板排程任務 |
| 系統監控 | **gopsutil** | 讀 /proc | 跨平台 CPU/記憶體/磁碟/程序 |
| Docker | 官方 SDK | — | 容器/映像/磁碟區/網路 |
| 日誌 | zerolog | zap | 輕量、結構化、低分配 |
| 認證 | JWT + refresh | session | 無狀態 API + 可選會話 |
| 2FA | TOTP（RFC 6238） | — | 復用 pquerna/otp |
| AI | 抽象 LLM 介面 | — | OpenAI/DeepSeek/Qwen/Ollama 統一適配 |

### 1.3 總體架構

```
┌────────────────────────────────────────────────────────────┐
│                OpsMini（单机面板，单二进制）                    │
│                                                            │
│   Vue3 SPA（构建产物 embed 进二进制）                          │
│        │  HTTP / WebSocket                                  │
│   ┌────▼────────────────────────────────────────────────┐   │
│   │  Gin Router → 中间件（认证/权限/审计/限流/安全入口）     │   │
│   │  Controller（参数校验）→ Service（业务）→ Repo（GORM）  │   │
│   └────┬────────────────────────────────────────────────┘   │
│        │                                                    │
│   ┌────▼────────────────────────────────────────────────┐   │
│   │  SQLite（业务数据）   │   系统资源适配层                │   │
│   │  users/cron/websites │   Docker SDK / crontab /       │   │
│   │  ...                 │   gopsutil / 文件系统 / SSH    │   │
│   └──────────────────────┴───────────────────────────────┘   │
│                                                              │
│   对外暴露两套标准 REST API：                                   │
│   · /api/v1   面板 API（浏览器，用户 JWT + RBAC）              │
│   · /agent/v1 Agent API（机器，Agent Token，命令白名单）       │
└──────────────────────────────────────────────────────────────┘
```

---

## 2. 核心架構決策

### 2.1 單體單一二進位，不做微服務

**決策**：單一程序單體。前端 Vue3 建置產物透過 `go:embed` 嵌入二進位，最終交付一個 `opsmini` 可執行檔。

**理由**：
- 維運面板的核心訴求是「裝一台、管一台」，單體最貼合；
- 對外整合透過標準 REST API 完成，與單體不衝突；
- 微服務會帶來部署、服務發現、分散式交易等無謂複雜度。

### 2.2 前後端分離 + 嵌入打包

- 開發期：Vue3 dev server 代理到 Gin（跨域 / 反代）；
- 生產期：`web/dist` 產物 `go:embed` 進二進位，單檔部署。

### 2.3 SQLite 而非 MySQL/Postgres

- 業務資料量小（設定、任務、使用者、日誌），SQLite 完全夠用；
- 零維運、單檔、易備份（直接拷貝 .db）；
- 若未來面板自身資料需要高並發，可抽象 Repository 介面切換。

### 2.4 對外暴露標準 REST API

- v1.0 **不開發 Proxy 中心節點**，僅把面板能力暴露為標準 REST API；
- 兩套 API 並存：`/api/v1`（面向瀏覽器 UI）與 `/agent/v1`（面向機器/第三方整合）；
- Agent API 獨立認證（Agent Token）+ 命令白名單，與使用者會話 JWT 隔離；
- 未來若需多機統一管理，可在 Agent API 之上加一層 Proxy 拉取排程（不在 v1.0 範圍）。

---

## 3. 模組劃分（對應 UI 原型）

| 後端模組 | 職責 | 對應原型頁面 |
|----------|------|-------------|
| `auth` | 登入/登出、會話、2FA、RBAC | 登入頁、使用者管理 |
| `setting` | 面板設定、主題、選單顯隱、語言 | 面板設定（基礎/外觀/選單） |
| `dashboard` | 指標聚合、即時推送 | 儀表板 |
| `monitor` | 時序採集、告警規則、告警觸發 | 監控 |
| `website` | Nginx 站點、網域、SSL 憑證 | 應用管理-網站 |
| `database` | MySQL/PostgreSQL 實例與庫 | 應用管理-資料庫 |
| `store` | 軟體安裝/移除/升級 | 應用管理-軟體商店 |
| `container` | Docker 容器/映像/磁碟區/網路 | 容器管理 |
| `system` | 程序/網路/連接埠/磁碟 | 系統管理 |
| `file` | 檔案瀏覽/上傳/編輯/權限 | 檔案 |
| `terminal` | Web SSH | 終端機 |
| `cron` | 排程任務（面板+系統+使用者 crontab） | 排程任務 |
| `log` | 日誌收集/聚合/tail | 日誌 |
| `ai` | LLM 接入、上下文、NL 執行 | AI 助手、面板設定-AI |
| `agent` | 對外 REST API（`/agent/v1`）、Agent Token 認證、命令白名單 | 面板設定-Proxy 接入 |

---

## 4. 分層與目錄結構

### 4.1 分層

每個模組內部嚴格三層：

```
Controller（HTTP handler）：参数解析校验、调用 Service、组装响应
    → Service（业务逻辑）：编排 Repository 与系统资源、事务边界
    → Repository（GORM）：纯数据访问，不写业务
```

### 4.2 目錄結構

```
opsmini/
├── cmd/
│   └── agent/main.go          # 单机面板主程序
├── internal/
│   ├── router/                # 路由注册、API 版本
│   ├── middleware/            # 认证、权限、审计、限流、安全入口
│   ├── api/v1/                # Controller（按模块分包，含 /api/v1 与 /agent/v1）
│   ├── service/               # Service（按模块分包）
│   ├── repository/            # Repository（按模块分包）
│   ├── model/                 # GORM 数据模型
│   ├── agent/                 # Agent REST API（对外暴露、token 认证、命令白名单）
│   ├── ai/                    # LLM 适配层（provider 接口 + 各实现）
│   └── pkg/                   # 通用工具
│       ├── config/            # 配置加载（文件+环境变量）
│       ├── logger/            # zerolog 封装
│       ├── jwt/               # token 签发/校验
│       ├── otp/               # 2FA TOTP
│       ├── sysinfo/           # gopsutil 封装（采集指标）
│       ├── crontab/           # 系统/用户 crontab 读写
│       ├── docker/            # Docker SDK 封装
│       └── store/             # SQLite 连接 + 迁移
├── web/                       # Vue3 前端源码（构建后 embed）
├── docs/
├── configs/                   # 默认配置示例
└── go.mod
```

---

## 5. 資料模型（SQLite Schema）

> GORM 遷移；敏感欄位（金鑰/Token）用 AES-GCM 加密後落庫。

```sql
-- 用户与认证
CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,          -- bcrypt
  role          TEXT NOT NULL DEFAULT 'operator',  -- admin/operator/readonly
  auth_method   TEXT NOT NULL DEFAULT 'password',  -- password/2fa
  totp_secret   TEXT,                    -- 加密存储
  status        INTEGER NOT NULL DEFAULT 1,        -- 1启用 0停用
  last_login    TEXT,
  created_at    TEXT,
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL,
  refresh    TEXT NOT NULL UNIQUE,
  expire_at  TEXT NOT NULL,
  created_at TEXT
);

-- 面板配置（KV，含主题/语言/菜单显隐/代理）
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT
);

-- 网站
CREATE TABLE websites (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  domain     TEXT NOT NULL UNIQUE,
  path       TEXT NOT NULL,
  env        TEXT,                       -- nginx/static
  runtime    TEXT,                       -- php8.2/php8.1/node20/static
  ssl        INTEGER DEFAULT 0,
  ssl_days   INTEGER,
  status     INTEGER DEFAULT 1,
  created_at TEXT
);

-- 数据库实例
CREATE TABLE databases (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  type      TEXT NOT NULL,               -- mysql/postgresql
  name      TEXT NOT NULL,
  charset   TEXT,
  created_at TEXT
);

-- 计划任务（含系统/用户 crontab 的只读映射）
CREATE TABLE cron_jobs (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  name      TEXT NOT NULL,
  kind      TEXT NOT NULL,               -- panel/system/user
  user      TEXT,
  src_path  TEXT,                        -- /etc/crontab 或 sqlite 等
  schedule  TEXT NOT NULL,               -- cron 表达式
  command   TEXT NOT NULL,
  enabled   INTEGER DEFAULT 1,
  last_run  TEXT,
  created_at TEXT
);

-- 告警规则
CREATE TABLE alert_rules (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  name      TEXT NOT NULL,
  metric    TEXT NOT NULL,               -- cpu/mem/disk/service
  condition TEXT NOT NULL,               -- >90% 等
  duration  TEXT,
  notify    TEXT,
  enabled   INTEGER DEFAULT 1,
  created_at TEXT
);

-- Agent API 配置
CREATE TABLE agent_config (
  id        INTEGER PRIMARY KEY CHECK (id = 1),  -- 单行
  agent_id  TEXT,
  token     TEXT,                        -- 加密存储（对外 REST API 认证）
  allowed_commands TEXT,                 -- 命令白名单（逗号分隔）
  enabled   INTEGER DEFAULT 0
);

-- 操作/审计日志
CREATE TABLE audit_logs (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER,
  action     TEXT,
  target     TEXT,
  detail     TEXT,
  created_at TEXT
);
```

---

## 6. API 設計（RESTful，`/api/v1`）

統一回應：`{ "code": 0, "message": "ok", "data": ... }`，`code` 非 0 為業務錯誤。

| 方法 | 路徑 | 說明 |
|------|------|------|
| POST | `/auth/login` | 登入（回傳 access + refresh） |
| POST | `/auth/refresh` | 重新整理 token |
| POST | `/auth/logout` | 登出 |
| GET | `/auth/2fa/qrcode` | 產生 2FA 二維碼 |
| POST | `/auth/2fa/verify` | 驗證 2FA |
| GET | `/users` / POST / PUT / DELETE | 使用者 CRUD |
| GET | `/dashboard/metrics` | 儀表板指標 |
| GET | `/monitor/series?range=1h` | 時序資料 |
| CRUD | `/alert-rules` | 告警規則 |
| CRUD | `/websites` | 網站 |
| POST | `/websites/:id/ssl` | 簽發/續期 SSL |
| CRUD | `/databases` | 資料庫 |
| GET | `/store/apps` / POST `/store/apps/:id/install` | 軟體商店 |
| GET | `/containers` / `/images` / `/volumes` / `/networks` | 容器四類 |
| POST | `/containers` 等 | 新建容器/拉映像/建磁碟區/建網路 |
| GET | `/system/processes` `/networks` `/ports` `/disks` | 系統資源 |
| GET/POST | `/files` / `/files/list` / `/files/upload` / `/files/edit` | 檔案 |
| WS  | `/terminal/ws?cols=&rows=` | Web SSH |
| CRUD | `/cron-jobs` | 排程任務（面板類） |
| GET | `/cron-jobs/system` `/cron-jobs/user` | 系統/使用者 crontab 唯讀 |
| GET | `/logs` | 日誌列表 |
| POST | `/ai/chat` | AI 對話（串流 SSE） |
| POST | `/ai/execute` | NL 轉操作（帶權限確認） |
| GET/PUT | `/agent/config` | Agent API 設定（token、命令白名單） |
| GET/PUT | `/settings` | 面板設定 |
| GET | `/i18n/{lang}` | 語言包（前端也可內聯） |
| GET | `/metrics` | **Prometheus 指標**（node_exporter 相容，無 `/api/v1` 前綴） |

### 6.1 Prometheus 監控整合

OpsMini 內建 **node_exporter 相容的 `/metrics` 端點**，無需額外安裝 node_exporter，Prometheus 可直接 scrape：

```
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
```

已對齊 node_exporter 的核心指標（可直接套用社群 Node Dashboard）：

| 指標族 | 說明 |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | 每核各模式累計秒數 |
| `node_memory_MemTotal_bytes` 等 | 記憶體 Total/Free/Available/Buffers/Cached |
| `node_filesystem_size_bytes{mountpoint}` | 各掛載點容量/可用/使用率 |
| `node_network_receive_bytes_total{device}` | 各網卡收發流量 |
| `node_load1` / `node_load5` / `node_load15` | 負載 |
| `node_uname_info` / `node_boot_time_seconds` | 主機資訊與啟動時間 |

> 實作方式：復用 `gopsutil`（SystemService 已採集的資料），按 Prometheus 文字格式輸出，
> 保持單一二進位交付，不嵌入 node_exporter 程序。

### 6.2 Agent API（對外的標準 REST 介面）

面板除了 UI 使用的 `/api/v1`，還暴露一組**獨立的機器對機器 REST API**（`/agent/v1`），供外部系統（監控平台、自動化腳本、第三方編排工具）整合呼叫。與面板 API 的區別：

- **認證**：用 Agent Token，而非使用者會話 JWT；
- **範圍**：聚焦資源管理與命令執行，不含 UI 專屬能力（i18n/主題/選單）與終端機 WS；
- **風格**：面向自動化——冪等、可重試、統一 JSON 回應。

| 方法 | 路徑 | 說明 |
|------|------|------|
| GET | `/agent/v1/health` | 健康檢查（探活） |
| GET | `/agent/v1/status` | 狀態摘要：cpu/mem/磁碟/線上服務 |
| GET | `/agent/v1/system/info` | 主機資訊（hostname/os/核心） |
| GET | `/agent/v1/system/processes` | 程序列表 |
| GET | `/agent/v1/system/ports` | 連接埠監聽 |
| GET | `/agent/v1/system/disks` | 磁碟/掛載點 |
| GET | `/agent/v1/websites` / POST | 網站查詢/建立 |
| GET | `/agent/v1/databases` | 資料庫列表 |
| GET | `/agent/v1/containers` | 容器列表 |
| GET | `/agent/v1/cron-jobs` | 排程任務 |
| POST | `/agent/v1/commands` | 執行命令（白名單，回傳執行結果） |

---

## 7. 對外 REST API 設計（差異化重點）

### 7.1 定位

v1.0 的 OpsMini 是**單機面板**，透過標準 REST API 對外暴露能力，供外部系統整合：

- **`/api/v1`（面板 API）**：面向瀏覽器 UI，使用者會話 JWT + RBAC；
- **`/agent/v1`（Agent API）**：面向機器/第三方整合，Agent Token 認證 + 命令白名單。

> 與 salt minion/master 的「出站長連線 push」不同，OpsMini 直接**暴露 REST 供外部拉取呼叫**，
> 更接近 Prometheus 拉 exporter / 雲廠商 OpenAPI 的思路。是否引入 Proxy 中心做多機統一管理，留待後續版本評估（v1.0 不做）。

### 7.2 呼叫模型

```
         HTTPS REST 调用（Agent Token）
   ┌──────────┐  ─────────────────────▶  ┌─────────┐
   │ 外部系统  │                          │ OpsMini │
   │ (监控/脚本 │  ◀─────────────────────  │ (单机面板)│
   │ /编排工具) │       统一 JSON 响应      └─────────┘
   └──────────┘
```

- **無長連線**：全部走標準 REST，不維持 WebSocket / gRPC 長連線；
- **串流場景**（日誌 tail、即時指標）：REST 分頁輪詢即可，無需 SSE；
- **冪等**：查詢類 GET 天然冪等；寫操作（命令執行、資源建立）回傳明確結果。

### 7.3 關鍵流程

1. 面板啟動 → 讀 `agent_config`（token + 命令白名單）；
2. 外部系統攜帶 `Authorization: Bearer <token>` 調 `/agent/v1/*`；
3. 中介軟體校驗 token（恆定時間比較）→ 命中白名單放行，否則 401；
4. 高危操作（命令執行）走命令白名單二次校驗，全部記審計；
5. 回傳統一 JSON：`{ code, message, data }`。

### 7.4 安全

- **認證**：面板配獨立隨機 Agent Token（加密儲存），請求標頭 `Authorization: Bearer <token>`；
- **傳輸**：HTTPS；高安全場景可加 IP 白名單；
- **命令白名單**：`/commands` 僅允許顯式授權命令，全部記審計；
- **最小暴露**：`/agent/v1` 與 `/api/v1` 分離，Agent API 不暴露 UI 能力與終端機。

### 7.5 兩套 API 的定位

| 維度 | 面板 API `/api/v1` | Agent API `/agent/v1` |
|------|-------------------|----------------------|
| 使用者 | 瀏覽器（人） | 外部系統（機器） |
| 認證 | 使用者會話 JWT + RBAC | Agent Token |
| 範圍 | 全部 UI 功能 + 終端機 WS | 資源管理 + 命令執行（無 UI/終端機） |
| 設計 | 面向互動 | 面向自動化（冪等、可重試） |

---

## 8. 安全設計

| 維度 | 方案 |
|------|------|
| 密碼 | bcrypt 雜湊 |
| 會話 | 短期 access JWT（15min）+ refresh token（可撤銷） |
| 2FA | TOTP（RFC 6238），登入時可選二次驗證 |
| 授權 | RBAC 三角色：admin（全部）/ operator（日常維運）/ readonly（唯讀） |
| 安全入口 | 存取面板需帶 secret 路徑（如 `/opsmini_panel`），防連接埠掃描 |
| 金鑰儲存 | 面板金鑰（API Key、Agent Token）AES-GCM 加密落庫 |
| 終端機 | Web SSH 僅對 operator 及以上開放，會話記錄審計 |
| 防爆破 | 登入失敗限流 + 鎖定 |
| 審計 | 關鍵操作寫 `audit_logs` |
| CSRF/XSS | API 用 Bearer Token 無 Cookie；前端轉義 |

---

## 9. 關鍵流程

### 9.1 登入

```
输入账密 → bcrypt 校验 → 若开启 2FA 则要求 TOTP
  → 签发 access + refresh → 前端存 refresh（httpOnly/localStorage）
  → 后续请求带 Bearer access → 过期用 refresh 换新
```

### 9.2 Web SSH 終端機

```
前端 ws://host/api/v1/terminal/ws?token=...
  → 服务端校验 token + 角色
  → 启动 pty（github.com/creack/pty）→ 双向数据转发
  → 关闭时回收 pty、记录会话时长
```

### 9.3 排程任務執行

- **面板任務**：robfig/cron 常駐排程，寫 `cron_jobs`；
- **系統/使用者任務**：直接讀寫 `/etc/crontab`、`/etc/cron.d/`、`/var/spool/cron/<user>`（唯讀展示 + 受控編輯）。

### 9.4 AI 助手

```
用户输入 → Service 拼上下文（当前页面/模块 + 系统状态）
  → 调 LLM（provider 适配：OpenAI/DeepSeek/Qwen/Ollama）
  → 若模型判定为「执行意图」→ 生成结构化 action + 参数
  → 命中权限白名单 → 执行 → 回传结果
  → 未授权/高危 → 要求用户二次确认
```

### 9.5 指標採集

- 採集器：gopsutil 每 5s 採樣 → 記憶體 ring buffer；
- 歷史：降採樣後落 SQLite（1min 粒度保留 7 天）；
- 推送：WebSocket 廣播給儀表板/監控頁訂閱者。

---

## 10. 監控與可觀測性

- 結構化日誌（zerolog），級別分級，面板內「日誌」頁可檢視；
- 指標：面板自身 + 主機指標統一由 monitor 模組採集；
- 健康檢查：`/api/healthz` 回傳程序/DB/磁碟狀態。

---

## 11. 部署

### 11.1 產物

- `opsmini` 單一二進位（約 25~30MB，`-s -w` 壓縮後），內建前端、SQLite、靜態資源；
- 設定：`/etc/opsmini/config.yaml` 或環境變數；
- 預設連接埠 8888，資料目錄 `/var/lib/opsmini/`（opsmini.db）。

### 11.1.1 多架構發布（x64 + arm64）

發布需覆蓋 **Linux** 的 **x86_64（amd64）** 與 **ARM64（arm64）** 兩套指令集，另編譯 macOS 目標僅用於開發除錯。

| 目標平台 | 適用場景 |
|----------|----------|
| `linux/amd64` | 主流 x64 伺服器（Intel/AMD，雲廠商通用機型） |
| `linux/arm64` | ARM 伺服器（Graviton、樹莓派、鯤鵬、飛騰等） |
| `darwin/arm64` | Apple Silicon 開發機（本機除錯） |
| `darwin/amd64` | Intel Mac（開發除錯） |

> OpsMini 面向 **Linux 主機**，僅發布 Linux 安裝包；macOS 目標僅用於開發除錯，不作為交付產物。

**關鍵前提**：資料層採用純 Go 驅動 `glebarez/sqlite`（底層 `modernc.org/sqlite`），**無 CGO 依賴**。因此 `CGO_ENABLED=0` 即可在任一平台一鍵交叉編譯出**靜態連結**的二進位，無需為每個架構準備 C 交叉工具鏈。

**建置方式**（`Makefile` 已就緒）：

```bash
make build        # 当前平台
make build-all    # 全平台交叉编译 → dist/
```

產物命名：`opsmini-<version>-<os>-<arch>`，版本號經 `-ldflags -X main.version` 注入，執行時 `opsmini -version` 可查。

已實測通過：4 個目標平台全部編譯成功，`file` 校驗架構正確（ELF x86-64 / ELF aarch64 / Mach-O arm64 / Mach-O x86_64），且均為靜態連結。

### 11.2 服務化

```ini
[Unit]
Description=OpsMini Agent
After=network.target

[Service]
ExecStart=/usr/local/bin/opsmini agent --config /etc/opsmini/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### 11.3 反向代理（可選）

Nginx 反代 80/443 → 8888，憑證由面板內網站模組管理或外部負載均衡承擔。

---

## 12. 開發路線（里程碑）

| 階段 | 內容 | 交付 |
|------|------|------|
| M1 骨架 | Go 工程、Gin 路由、SQLite、GORM 遷移、登入/使用者/RBAC | 可跑的最小面板 |
| M2 系統能力 | gopsutil 指標、程序/連接埠/磁碟/網路、儀表板+監控 | 單機監控閉環 |
| M3 資源管理 | 網站(Nginx)、資料庫、檔案、終端機(pty)、排程任務 | 對標 1Panel 核心 |
| M4 容器 | Docker SDK：容器/映像/磁碟區/網路 | 容器管理 |
| M5 AI | LLM 適配層、上下文、NL 執行、AI 助手 | 差異化能力 |
| M6 Agent API | Agent 暴露標準 REST API（`/agent/v1`）、token 認證、命令白名單 | 對外整合能力 |
| M7 打磨 | i18n、審計、限流、測試、文件 | 生產就緒 |

---

## 13. 待確認的決策點

1. **Agent API 認證強度**：預設 Bearer Token；是否需要上 mTLS 雙向憑證（安全更高、部署更重）？
2. **Agent API 是否需要 IP 白名單**：預設關閉，僅靠 Token；多機/公網場景是否加 IP 限制？
3. **串流資料**：日誌 tail / 即時指標，預設 REST 分頁輪詢，是否可接受？（v1.0 不引入 SSE/長連線）
4. **多機統一管理（未來）**：若後續引入 Proxy 中心，是復用現有 `/agent/v1` 做拉取排程，還是另設協定？（v1.0 範圍外，僅備忘）
