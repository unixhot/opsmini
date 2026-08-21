<div align="center">

# OpsMini Developer Guide

**開發者手冊**

[English](developer-guide.md) · [简体中文](developer-guide.zh-CN.md) · [繁體中文](developer-guide.zh-TW.md) · [日本語](developer-guide.ja.md) · [한국어](developer-guide.ko.md) · [ไทย](developer-guide.th.md) · [Deutsch](developer-guide.de.md)

</div>

---

> 版本：v1.0.0 · 語言：繁體中文 · [English](developer-guide.md)

本手冊深入說明 OpsMini 程式碼庫：架構、目錄結構、各模組職責、API 設計、資料模型、RBAC 權限體系，以及如何擴充。

---

## 1. 專案概述

OpsMini 是一款**輕量級單一主機伺服器管理面板**（對標寶塔 / 1Panel），兩大核心差異化：

1. **AI 大模型維運** — 透過 LLM 配接層，以自然語言管理系統（診斷、分析日誌、執行指令）。
2. **標準 REST 契約** — 面板暴露 `/api/v1`（瀏覽器 UI 使用）與 `/agent/v1`（機器對機器），便於中心 Proxy 拉取指標並統一管理多台主機。

交付形態為**單一靜態二進位檔**：Vue3 前端透過 `go:embed` 內嵌，SQLite 使用純 Go 驅動（無 CGO），因此 `CGO_ENABLED=0` 可交叉編譯至 linux/amd64、linux/arm64、darwin、windows。

### 技術堆疊

| 層級 | 技術 |
|------|------|
| 後端 | Go、Gin、GORM、`glebarez/sqlite`（純 Go）、gopsutil/v4、JWT（golang-jwt/v5）、bcrypt、robfig/cron/v3 |
| 前端 | Vue 3（單檔案 SPA）、ECharts、xterm.js（Web 終端機） |
| 儲存 | SQLite |
| 交付 | 單一二進位檔（`go:embed` 內嵌前端） |

---

## 2. 架構

後端採用清晰的分層架構：

```
HTTP 請求
   │
   ▼
router（gin.Engine，路由註冊 + 中介軟體裝配）
   │
   ▼
middleware（驗證 → RBAC 權限 → 稽核 → 存取日誌）
   │
   ▼
api/v1 handler（解析/驗證請求，呼叫 service，寫入回應）
   │
   ▼
service（業務邏輯、編排、外部系統呼叫）
   │
   ▼
repository（GORM 資料存取）
   │
   ▼
model（映射至 SQLite 資料表的結構體）
```

規則：

- **handler** 只做請求解析/驗證與回應封裝，不含業務邏輯。
- **service** 承載業務邏輯，編排 repository 與系統資源（gopsutil、Docker SDK、crontab、pty）。
- **repository** 封裝 GORM，是唯一接觸資料庫的層。
- **model** 是帶資料表映射與 JSON tag 的 GORM 結構體。

### 請求流程（面板 API）

```
瀏覽器 ──► /api/v1/xxx
             │  authMw      （解析 JWT，注入使用者 id/role）
             │  permMw      （RBAC 權限檢查，依路由選用）
             │  auditMw     （記錄稽核日誌）
             │  accessLogMw （記錄存取日誌）
             ▼
          handler → service → repository → SQLite
```

Agent API（`/agent/v1/*`）使用**獨立的 Agent Token** 驗證（非使用者 JWT），由 `middleware.AgentAuth` 守護。

---

## 3. 目錄結構

```
opsmini/
├── cmd/
│   └── agent/main.go        # 入口：解析參數、載入設定、初始化 DB、啟動 HTTP
├── configs/
│   └── config.yaml          # 預設設定範本
├── internal/
│   ├── config/              # 設定載入（Config 結構 + YAML + 預設值）
│   ├── router/              # gin 路由、路由註冊、中介軟體裝配
│   ├── middleware/          # auth、perm(RBAC)、audit、accesslog、cors、ratelimit、metrics_auth、agent
│   ├── api/v1/              # HTTP handler（面板 + agent 端點）
│   ├── service/             # 業務邏輯（每個模組一個檔案）
│   ├── repository/          # GORM 資料存取（每個 model 一個檔案）
│   ├── model/               # GORM 模型 + RBAC 權限分組
│   └── pkg/
│       ├── jwt/             # JWT 簽發/驗證（access + refresh）
│       ├── response/        # 統一 API 回應封裝
│       └── store/           # SQLite 初始化、自動遷移、種子資料
├── web/
│   ├── index.html           # 單檔案 Vue3 SPA（7 語言 i18n、主題系統）
│   ├── embed.go             # go:embed 將前端內嵌進二進位檔
│   └── static/              # 前端離線依賴
├── install/                 # 安裝腳本
├── docs/                    # 專案文件
└── Makefile                 # 建置 / 交叉編譯 / 版本目標
```

---

## 4. 模組說明

### 4.1 `cmd/agent/main.go`

入口。職責：

- 解析參數（`-config`、`-version`）；
- 透過 `internal/config` 載入設定；
- 透過 `internal/pkg/store` 開啟 SQLite；
- 初始化內建角色與預設 admin 帳號；
- 組裝 service + handler 交給 `internal/router`；
- 啟動 HTTP 服務（及可選的 metrics 端點）。

### 4.2 `internal/config`

`config.go` 定義 `Config` 結構，從 `-config` 路徑載入 YAML；檔案不存在時回傳內建預設值。設定段：`server`、`database`、`jwt`、`ai`、`agent`。

### 4.3 `internal/router`

`router.go` 是所有路由註冊的唯一入口：

- 公開路由（`/healthz`、`/auth/login`、`/auth/refresh` 等）；
- 已驗證面板路由（`/api/v1/*`），掛在 `authMw + audit + accesslog` 之後；
- 寫入路由額外加 `permMw("permission.key")` 守衛；
- agent 路由（`/agent/v1/*`），掛在 `middleware.AgentAuth` 之後；
- WebSocket 路由（`/terminal`、`/containers/:id/exec`）。

**約定：** 每個寫入端點（POST/PUT/DELETE）必須帶 `permMw(...)` 守衛。讀取端點對任意已登入使用者開放（除非涉及敏感資料）。

### 4.4 `internal/middleware`

| 檔案 | 作用 |
|------|------|
| `auth.go` | JWT 驗證；向上下文注入使用者 id/role |
| `perm.go` | RBAC 權限檢查（`permMw`） |
| `audit.go` | 寫入稽核日誌 |
| `accesslog.go` | 寫入存取日誌 |
| `cors.go` | CORS 標頭 |
| `ratelimit.go` | 登入限流 |
| `metrics_auth.go` | `/metrics` 的 Bearer Token 守衛 |
| `agent.go` | `/agent/v1` 的 Agent Token 驗證 |

### 4.5 `internal/api/v1`

每個模組一個 handler 檔案。每個 handler：

1. 綁定/驗證請求；
2. 呼叫對應 service 方法；
3. 回傳統一的 `response.OK` / `response.Error`。

面板 handler 位於 `v1` 套件（如 `system.go`、`file.go`、`skill.go`）；agent handler 位於 `agent.go`。

### 4.6 `internal/service`

業務邏輯層，每個模組一個檔案。主要模組：

| 檔案 | 模組 |
|------|------|
| `auth.go`、`user.go`、`role.go`、`totp.go` | 驗證、使用者、RBAC、雙因素驗證 |
| `system.go`、`metrics.go`、`prometheus.go` | 主機資訊、指標、prometheus 匯出 |
| `website.go`、`database.go`、`cron.go`、`crontab.go` | 資源管理 |
| `file.go` | 檔案操作（含目錄穿越防護） |
| `docker.go` | Docker 容器/映像/磁碟區/網路 |
| `ai.go` | LLM 配接器（對話、串流） |
| `skill.go`、`mcp.go` | AI 技能 + MCP 服務 |
| `alert.go`、`alertmonitor.go` | 警示規則 + 評估 |
| `security.go`、`securitymonitor.go`、`baseline.go`、`fim.go`、`threat.go`、`firewall.go`、`loginsecurity.go` | 主機安全套件 |
| `notification.go`、`audit.go`、`setting.go` | 通知、稽核、設定 |
| `appstore.go`、`apptemplate.go`、`appcategory.go` | 應用程式商店 / 範本 |

### 4.7 `internal/repository`

GORM 資料存取，每個 model 一個檔案，提供 CRUD 與查詢輔助。不含業務邏輯。

### 4.8 `internal/model`

映射至 SQLite 資料表的 GORM 結構體，其中 `role.go` 是 RBAC 權限分組的唯一權威來源（`PermGroups()`、`AllPermKeys()`、`BuiltinRoles()`）。

### 4.9 `internal/pkg`

| 套件 | 作用 |
|------|------|
| `jwt` | access/refresh token 簽發與驗證 |
| `response` | 統一回應封裝 `{code,message,data}` |
| `store` | SQLite 開啟、自動遷移、種子資料（預設 admin） |

### 4.10 `web`

- `index.html` — 單檔案 Vue3 SPA：7 語言 i18n、主題系統、登入、儀表板、監控、安全、檔案、終端機、AI 助理、設定。
- `embed.go` — `go:embed` 將前端內嵌進二進位檔。
- `static/` — 前端離線依賴（Vue、ECharts）。

---

## 5. API 設計

### 5.1 回應封裝

每個端點回傳：

```json
{ "code": 0, "message": "ok", "data": { } }
```

- `code == 0` → 成功，`data` 承載資料；
- `code != 0` → 業務錯誤，`message` 描述錯誤。

### 5.2 驗證

- 面板 API（`/api/v1`）：JWT access token（`Authorization: Bearer <token>`），短期有效，透過 `/auth/refresh` 重新整理。
- Agent API（`/agent/v1`）：靜態 Agent Token（設定 `agent.token`）。

### 5.3 端點族

| 族 | 受眾 | 驗證 |
|----|------|------|
| `/api/v1/*` | 瀏覽器 UI | 使用者 JWT + RBAC |
| `/agent/v1/*` | 外部系統 / Proxy | Agent Token |

---

## 6. 資料模型

模型是 `internal/model` 裡的 GORM 結構體，啟動時由 `internal/pkg/store` 自動遷移建表。代表性模型：

- `User`（id、username、密碼雜湊、role、MFA 金鑰等）
- `Role`（name、label、perms、builtin）
- `Website`、`Database`、`CronJob`
- `AlertRule`、`AlertEvent`、`Notification`
- `McpServer`、`Skill`（技能以磁碟目錄儲存，非 DB）
- `AuditLog`、`AccessLog`、`Setting`
- 安全：`BaselineResult`、`FimBaseline`、`FimChange`、`ThreatFinding`

---

## 7. RBAC 與權限

權限分組在 `internal/model/role.go`（`PermGroups()`）中定義，是唯一權威來源。三個內建角色：

- **admin** — 權限 `"*"`（全部）；
- **operator** — 除 `user.*` 與 `settings.edit` 外的所有權限；
- **readonly** — 僅 `*.view` 檢視權限。

前端選單/按鈕用 `hasPerm('key')` 做同一套 key 的顯示控制；後端用 `permMw("key")` 強制檢查。新增寫入功能時**必須**三處同時修改：

1. 在 `PermGroups()` 增加權限 key；
2. 路由加 `permMw(...)` 守衛；
3. 前端按鈕加 `hasPerm(...)` 守衛。

---

## 8. 建置與部署

```bash
make build          # 目前平台建置
make build-all      # 交叉編譯所有平台
make version        # 列印版本資訊
```

二進位檔為靜態（無 CGO）。systemd、反向代理、升級說明見 [`build-and-deploy.md`](build-and-deploy.md)。

---

## 9. 開發指南

### 9.1 新增模組

遵循分層模式，建立（或擴充）：

1. `internal/model/xxx.go` — GORM 結構；
2. `internal/repository/xxx.go` — 資料存取；
3. `internal/service/xxx.go` — 業務邏輯；
4. `internal/api/v1/xxx.go` — handler；
5. 在 `internal/router/router.go` 註冊路由。

### 9.2 新增寫入端點

1. 在 `internal/model/role.go` 增加權限 key；
2. 用 `permMw("...")` 註冊路由；
3. 前端按鈕加 `hasPerm("...")` 守衛；
4. 在 `web/index.html` 補 i18n key（7 語言全）。

### 9.3 i18n 規範

前端 i18n 字典（`I18N`...`I18N8`）含 7 語言：`zh-CN`、`zh-TW`、`en`、`ja`、`ko`、`th`、`de`。每個新 key 必須補全**全部 7 語言**——`t(key)` 會在缺失時回退至 `zh-CN`，但缺失翻譯會讓非中文使用者看到中文。

### 9.4 程式碼風格

- 匯出的 Go 函式/型別帶以其名稱開頭的文件註解。
- 註解統一使用英文。
- 每個目錄都有描述其檔案的 `README.md`。
