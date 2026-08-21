<div align="center">

# OpsMini Backend Architecture

**Backend Architecture Design**

[English](backend-architecture.md) · [简体中文](backend-architecture.zh-CN.md) · [繁體中文](backend-architecture.zh-TW.md) · [日本語](backend-architecture.ja.md) · [한국어](backend-architecture.ko.md) · [ไทย](backend-architecture.th.md) · [Deutsch](backend-architecture.de.md)

</div>

---

> Version: v1.0  
> Based on: the `web/index.html` UI prototype (Login / Users / Dashboard / Monitoring / Apps / Containers / System / Files / Terminal / Cron Jobs / Logs / AI Assistant / Panel Settings / 7-language i18n)

---

## 1. Overview

OpsMini is a **lightweight single-machine operations panel** (comparable to BaoTa / 1Panel) with two key differentiators:

1. **Built-in AI large model** for system management (natural-language diagnostics / execution / log analysis);
2. **Exposes standard REST APIs** that can be integrated and called by external systems (monitoring platforms, automation scripts, third-party orchestration tools).

> v1.0 scope note: this version does **not include the Proxy central node**. The OpsMini panel on each host works independently,
> exposing capabilities to the outside via standard REST APIs; unified multi-machine management (Proxy) is deferred for a later version.

### 1.1 Design Goals

| Goal | Description |
|------|------|
| Lightweight | Single-binary deployment, low memory footprint, suitable for small 1C1G hosts |
| Single-machine first | The core scenario is a single server; no distributed complexity is introduced |
| Integratable | Exposes capabilities via standard REST APIs for integration by external systems |
| Secure | RBAC, 2FA, secure entry, least privilege, encrypted key storage |
| Maintainable | Modular layering, with clear Controller → Service → Repository separation |

### 1.2 Technology Stack

| Layer | Choice | Alternative | Rationale |
|----|------|------|------|
| Language | Go 1.22+ | — | Single binary, cross-compilation, good concurrency, mature ecosystem (same stack as 1Panel) |
| Web framework | **Gin** | Echo / chi | Largest ecosystem, rich middleware, same as 1Panel |
| Database | **SQLite** | — | Embedded, zero-ops, fits the single-machine scenario |
| SQLite driver | **modernc.org/sqlite** | mattn/go-sqlite3 | Pure Go without CGO, easy cross-compilation |
| ORM | **GORM** | sqlx | High development efficiency; complex queries can fall back to raw SQL |
| Real-time communication | **gorilla/websocket** | — | Terminal, log tail, metric push |
| Task scheduling | **robfig/cron** | — | Panel cron jobs |
| System monitoring | **gopsutil** | read /proc | Cross-platform CPU/memory/disk/process |
| Docker | Official SDK | — | Containers/images/volumes/networks |
| Logging | zerolog | zap | Lightweight, structured, low allocation |
| Authentication | JWT + refresh | session | Stateless API + optional session |
| 2FA | TOTP (RFC 6238) | — | Reuses pquerna/otp |
| AI | Abstract LLM interface | — | Unified adapters for OpenAI/DeepSeek/Qwen/Ollama |

### 1.3 Overall Architecture

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

## 2. Core Architecture Decisions

### 2.1 Monolith single binary, no microservices

**Decision**: single-process monolith. The frontend Vue3 build output is embedded into the binary via `go:embed`, and the final deliverable is a single `opsmini` executable.

**Rationale**:
- The core requirement of an operations panel is "install on one machine, manage that machine"; a monolith fits best;
- External integration is done via standard REST APIs, which does not conflict with a monolith;
- Microservices would introduce needless complexity such as deployment, service discovery, and distributed transactions.

### 2.2 Frontend/backend separation + embedded packaging

- Development: the Vue3 dev server proxies to Gin (CORS / reverse proxy);
- Production: the `web/dist` output is embedded into the binary via `go:embed`, for single-file deployment.

### 2.3 SQLite instead of MySQL/Postgres

- Business data volume is small (config, jobs, users, logs); SQLite is fully sufficient;
- Zero-ops, single file, easy backup (copy the .db directly);
- If the panel's own data later needs high concurrency, a Repository interface abstraction allows switching.

### 2.4 Exposing standard REST APIs

- v1.0 does **not develop a Proxy central node**; it only exposes panel capabilities as standard REST APIs;
- Two APIs coexist: `/api/v1` (for the browser UI) and `/agent/v1` (for machines/third-party integration);
- The Agent API uses independent authentication (Agent Token) + command allowlist, isolated from user-session JWTs;
- If unified multi-machine management is needed in the future, a Proxy pull/scheduling layer can be added on top of the Agent API (out of v1.0 scope).

---

## 3. Module Breakdown (mapped to the UI prototype)

| Backend module | Responsibility | Corresponding prototype page |
|----------|------|-------------|
| `auth` | Login/logout, sessions, 2FA, RBAC | Login page, User management |
| `setting` | Panel config, theme, menu visibility, language | Panel settings (Basic/Appearance/Menu) |
| `dashboard` | Metric aggregation, real-time push | Dashboard |
| `monitor` | Time-series collection, alert rules, alert triggering | Monitoring |
| `website` | Nginx sites, domains, SSL certificates | App management - Websites |
| `database` | MySQL/PostgreSQL instances and databases | App management - Databases |
| `store` | Software install/uninstall/upgrade | App management - Software store |
| `container` | Docker containers/images/volumes/networks | Container management |
| `system` | Processes/network/ports/disks | System management |
| `file` | File browsing/upload/edit/permissions | Files |
| `terminal` | Web SSH | Terminal |
| `cron` | Cron jobs (panel + system + user crontab) | Cron jobs |
| `log` | Log collection/aggregation/tail | Logs |
| `ai` | LLM integration, context, NL execution | AI Assistant, Panel settings - AI |
| `agent` | External REST API (`/agent/v1`), Agent Token auth, command allowlist | Panel settings - Proxy integration |

---

## 4. Layering and Directory Structure

### 4.1 Layering

Each module has a strict three-layer structure:

```
Controller（HTTP handler）：参数解析校验、调用 Service、组装响应
    → Service（业务逻辑）：编排 Repository 与系统资源、事务边界
    → Repository（GORM）：纯数据访问，不写业务
```

### 4.2 Directory Structure

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

## 5. Data Model (SQLite Schema)

> GORM migration; sensitive fields (keys/tokens) are encrypted with AES-GCM before being stored.

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

## 6. API Design (RESTful, `/api/v1`)

Unified response: `{ "code": 0, "message": "ok", "data": ... }`; a non-zero `code` indicates a business error.

| Method | Path | Description |
|------|------|------|
| POST | `/auth/login` | Login (returns access + refresh) |
| POST | `/auth/refresh` | Refresh token |
| POST | `/auth/logout` | Logout |
| GET | `/auth/2fa/qrcode` | Generate 2FA QR code |
| POST | `/auth/2fa/verify` | Verify 2FA |
| GET | `/users` / POST / PUT / DELETE | User CRUD |
| GET | `/dashboard/metrics` | Dashboard metrics |
| GET | `/monitor/series?range=1h` | Time-series data |
| CRUD | `/alert-rules` | Alert rules |
| CRUD | `/websites` | Websites |
| POST | `/websites/:id/ssl` | Issue/renew SSL |
| CRUD | `/databases` | Databases |
| GET | `/store/apps` / POST `/store/apps/:id/install` | Software store |
| GET | `/containers` / `/images` / `/volumes` / `/networks` | The four container resources |
| POST | `/containers` etc. | Create container / pull image / create volume / create network |
| GET | `/system/processes` `/networks` `/ports` `/disks` | System resources |
| GET/POST | `/files` / `/files/list` / `/files/upload` / `/files/edit` | Files |
| WS  | `/terminal/ws?cols=&rows=` | Web SSH |
| CRUD | `/cron-jobs` | Cron jobs (panel type) |
| GET | `/cron-jobs/system` `/cron-jobs/user` | System/user crontab (read-only) |
| GET | `/logs` | Log list |
| POST | `/ai/chat` | AI chat (streaming SSE) |
| POST | `/ai/execute` | NL-to-action (with permission confirmation) |
| GET/PUT | `/agent/config` | Agent API config (token, command allowlist) |
| GET/PUT | `/settings` | Panel settings |
| GET | `/i18n/{lang}` | Language pack (frontend can also inline it) |
| GET | `/metrics` | **Prometheus metrics** (node_exporter compatible, no `/api/v1` prefix) |

### 6.1 Prometheus Monitoring Integration

OpsMini has a built-in **node_exporter-compatible `/metrics` endpoint**, so no separate node_exporter is needed; Prometheus can scrape it directly:

```
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
```

The following core node_exporter metrics are aligned (the community Node Dashboard can be used directly):

| Metric family | Description |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | Cumulative seconds per core per mode |
| `node_memory_MemTotal_bytes` etc. | Memory Total/Free/Available/Buffers/Cached |
| `node_filesystem_size_bytes{mountpoint}` | Capacity/available/usage per mount point |
| `node_network_receive_bytes_total{device}` | RX/TX traffic per interface |
| `node_load1` / `node_load5` / `node_load15` | Load average |
| `node_uname_info` / `node_boot_time_seconds` | Host info and boot time |

> Implementation: reuse `gopsutil` (data already collected by SystemService), output in Prometheus text format,
> keeping single-binary delivery without embedding a node_exporter process.

### 6.2 Agent API (external standard REST interface)

Besides the `/api/v1` used by the UI, the panel exposes an **independent machine-to-machine REST API** (`/agent/v1`) for external systems (monitoring platforms, automation scripts, third-party orchestration tools) to integrate and call. Differences from the panel API:

- **Authentication**: uses Agent Token instead of user-session JWT;
- **Scope**: focused on resource management and command execution, excluding UI-specific capabilities (i18n/theme/menu) and terminal WS;
- **Style**: automation-oriented — idempotent, retryable, unified JSON responses.

| Method | Path | Description |
|------|------|------|
| GET | `/agent/v1/health` | Health check (liveness) |
| GET | `/agent/v1/status` | Status summary: cpu/mem/disk/online services |
| GET | `/agent/v1/system/info` | Host info (hostname/os/kernel) |
| GET | `/agent/v1/system/processes` | Process list |
| GET | `/agent/v1/system/ports` | Listening ports |
| GET | `/agent/v1/system/disks` | Disks/mount points |
| GET | `/agent/v1/websites` / POST | Website query/create |
| GET | `/agent/v1/databases` | Database list |
| GET | `/agent/v1/containers` | Container list |
| GET | `/agent/v1/cron-jobs` | Cron jobs |
| POST | `/agent/v1/commands` | Execute command (allowlisted, returns execution result) |

---

## 7. External REST API Design (differentiator focus)

### 7.1 Positioning

In v1.0, OpsMini is a **single-machine panel** that exposes capabilities via standard REST APIs for external systems to integrate:

- **`/api/v1` (Panel API)**: for the browser UI, user-session JWT + RBAC;
- **`/agent/v1` (Agent API)**: for machines/third-party integration, Agent Token auth + command allowlist.

> Unlike salt minion/master's "outbound long-lived connection push", OpsMini directly **exposes REST for external pull calls**,
> closer to the Prometheus-pulls-exporter / cloud-provider OpenAPI approach. Whether to introduce a Proxy center for unified multi-machine management is deferred for later evaluation (not in v1.0).

### 7.2 Call Model

```
         HTTPS REST 调用（Agent Token）
   ┌──────────┐  ─────────────────────▶  ┌─────────┐
   │ 外部系统  │                          │ OpsMini │
   │ (监控/脚本 │  ◀─────────────────────  │ (单机面板)│
   │ /编排工具) │       统一 JSON 响应      └─────────┘
   └──────────┘
```

- **No long-lived connections**: everything goes over standard REST, no WebSocket / gRPC long-lived connections;
- **Streaming scenarios** (log tail, real-time metrics): REST pagination polling suffices, no SSE needed;
- **Idempotency**: read-only GETs are naturally idempotent; write operations (command execution, resource creation) return explicit results.

### 7.3 Key Flows

1. Panel starts → reads `agent_config` (token + command allowlist);
2. External system calls `/agent/v1/*` with `Authorization: Bearer <token>`;
3. Middleware verifies the token (constant-time comparison) → allows if it matches the allowlist, otherwise 401;
4. High-risk operations (command execution) go through a second command-allowlist check, and everything is audited;
5. Returns unified JSON: `{ code, message, data }`.

### 7.4 Security

- **Authentication**: the panel configures an independent random Agent Token (encrypted storage), request header `Authorization: Bearer <token>`;
- **Transport**: HTTPS; in high-security scenarios an IP allowlist can be added;
- **Command allowlist**: `/commands` only permits explicitly authorized commands, all audited;
- **Minimal exposure**: `/agent/v1` is separated from `/api/v1`; the Agent API does not expose UI capabilities or the terminal.

### 7.5 Positioning of the two APIs

| Dimension | Panel API `/api/v1` | Agent API `/agent/v1` |
|------|-------------------|----------------------|
| Consumer | Browser (human) | External system (machine) |
| Authentication | User-session JWT + RBAC | Agent Token |
| Scope | All UI features + terminal WS | Resource management + command execution (no UI/terminal) |
| Design | Interaction-oriented | Automation-oriented (idempotent, retryable) |

---

## 8. Security Design

| Dimension | Approach |
|------|------|
| Password | bcrypt hashing |
| Session | Short-lived access JWT (15min) + refresh token (revocable) |
| 2FA | TOTP (RFC 6238), optional second factor at login |
| Authorization | RBAC with three roles: admin (all) / operator (daily ops) / readonly (read-only) |
| Secure entry | Accessing the panel requires a secret path (e.g. `/opsmini_panel`) to prevent port scanning |
| Key storage | Panel secrets (API Key, Agent Token) stored AES-GCM encrypted |
| Terminal | Web SSH only open to operator and above, session recorded and audited |
| Anti-brute-force | Login-failure rate limiting + lockout |
| Audit | Critical operations written to `audit_logs` |
| CSRF/XSS | API uses Bearer Token without cookies; frontend escapes output |

---

## 9. Key Flows

### 9.1 Login

```
输入账密 → bcrypt 校验 → 若开启 2FA 则要求 TOTP
  → 签发 access + refresh → 前端存 refresh（httpOnly/localStorage）
  → 后续请求带 Bearer access → 过期用 refresh 换新
```

### 9.2 Web SSH Terminal

```
前端 ws://host/api/v1/terminal/ws?token=...
  → 服务端校验 token + 角色
  → 启动 pty（github.com/creack/pty）→ 双向数据转发
  → 关闭时回收 pty、记录会话时长
```

### 9.3 Cron Job Execution

- **Panel jobs**: robfig/cron resident scheduling, written to `cron_jobs`;
- **System/user jobs**: directly read/write `/etc/crontab`, `/etc/cron.d/`, `/var/spool/cron/<user>` (read-only display + controlled editing).

### 9.4 AI Assistant

```
用户输入 → Service 拼上下文（当前页面/模块 + 系统状态）
  → 调 LLM（provider 适配：OpenAI/DeepSeek/Qwen/Ollama）
  → 若模型判定为「执行意图」→ 生成结构化 action + 参数
  → 命中权限白名单 → 执行 → 回传结果
  → 未授权/高危 → 要求用户二次确认
```

### 9.5 Metric Collection

- Collector: gopsutil samples every 5s → in-memory ring buffer;
- History: downsampled then stored in SQLite (1min granularity, 7-day retention);
- Push: WebSocket broadcast to dashboard/monitoring page subscribers.

---

## 10. Monitoring and Observability

- Structured logs (zerolog), leveled, viewable on the panel's "Logs" page;
- Metrics: panel's own + host metrics, collected uniformly by the monitor module;
- Health check: `/api/healthz` returns process/DB/disk status.

---

## 11. Deployment

### 11.1 Artifacts

- `opsmini` single binary (about 25~30MB after `-s -w` compression), with embedded frontend, SQLite, and static assets;
- Config: `/etc/opsmini/config.yaml` or environment variables;
- Default port 8888, data directory `/var/lib/opsmini/` (opsmini.db).

### 11.1.1 Multi-architecture releases (x64 + arm64)

Releases must cover **Linux** on both **x86_64 (amd64)** and **ARM64 (arm64)** instruction sets; a macOS target is compiled only for development/debugging.

| Target platform | Use case |
|----------|----------|
| `linux/amd64` | Mainstream x64 servers (Intel/AMD, common cloud-provider machines) |
| `linux/arm64` | ARM servers (Graviton, Raspberry Pi, Kunpeng, Phytium, etc.) |
| `darwin/arm64` | Apple Silicon development machines (local debugging) |
| `darwin/amd64` | Intel Mac (development/debugging) |

> OpsMini targets **Linux hosts**, so only Linux installers are released; the macOS target is only for development/debugging and is not a deliverable.

**Key prerequisite**: the data layer uses the pure-Go driver `glebarez/sqlite` (underlying `modernc.org/sqlite`) with **no CGO dependency**. Therefore `CGO_ENABLED=0` allows one-click cross-compilation of a **statically linked** binary on any platform, without preparing a C cross toolchain per architecture.

**Build method** (`Makefile` already in place):

```bash
make build        # 当前平台
make build-all    # 全平台交叉编译 → dist/
```

Artifact naming: `opsmini-<version>-<os>-<arch>`, with the version injected via `-ldflags -X main.version`, and viewable at runtime via `opsmini -version`.

Verified in practice: all 4 target platforms compile successfully, `file` confirms the correct architecture (ELF x86-64 / ELF aarch64 / Mach-O arm64 / Mach-O x86_64), and all are statically linked.

### 11.2 Service installation

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

### 11.3 Reverse proxy (optional)

Nginx reverse-proxies 80/443 → 8888, with certificates managed by the panel's website module or an external load balancer.

---

## 12. Development Roadmap (milestones)

| Phase | Content | Deliverable |
|------|------|------|
| M1 Skeleton | Go project, Gin routing, SQLite, GORM migration, login/users/RBAC | A minimal runnable panel |
| M2 System capabilities | gopsutil metrics, processes/ports/disks/network, dashboard + monitoring | Single-machine monitoring loop |
| M3 Resource management | Websites (Nginx), databases, files, terminal (pty), cron jobs | Parity with 1Panel core |
| M4 Containers | Docker SDK: containers/images/volumes/networks | Container management |
| M5 AI | LLM adapter layer, context, NL execution, AI assistant | Differentiating capability |
| M6 Agent API | Agent exposes standard REST API (`/agent/v1`), token auth, command allowlist | External integration capability |
| M7 Polish | i18n, audit, rate limiting, tests, docs | Production-ready |

---

## 13. Decision Points to Confirm

1. **Agent API authentication strength**: default Bearer Token; is mTLS mutual certificates needed (more secure, heavier deployment)?
2. **Does the Agent API need an IP allowlist**: disabled by default, token-only; should IP restrictions be added for multi-machine/public-network scenarios?
3. **Streaming data**: log tail / real-time metrics default to REST pagination polling — is this acceptable? (v1.0 does not introduce SSE/long-lived connections)
4. **Unified multi-machine management (future)**: if a Proxy center is introduced later, should it reuse the existing `/agent/v1` for pull scheduling, or define a separate protocol? (out of v1.0 scope, memo only)
