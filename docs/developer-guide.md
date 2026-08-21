<div align="center">

# OpsMini Developer Guide

**Developer Guide**

[English](developer-guide.md) · [简体中文](developer-guide.zh-CN.md) · [繁體中文](developer-guide.zh-TW.md) · [日本語](developer-guide.ja.md) · [한국어](developer-guide.ko.md) · [ไทย](developer-guide.th.md) · [Deutsch](developer-guide.de.md)

</div>

---

> Version: v1.0.0 · Language: English · [简体中文](developer-guide.zh-CN.md)

This guide explains the OpsMini codebase in depth: architecture, directory
layout, every module, API design, data model, RBAC, and how to extend it.

---

## 1. Overview

OpsMini is a **lightweight single-host server management panel** (similar to
BaoTa / 1Panel), differentiated by two things:

1. **AI-powered operations** — an LLM adapter lets you manage the system with
   natural language (diagnose, analyze logs, run commands).
2. **A clean REST contract** — the panel exposes `/api/v1` (for the browser UI)
   and `/agent/v1` (machine-to-machine), so a central proxy can pull metrics and
   manage many hosts.

It ships as a **single static binary**: the Vue3 frontend is embedded via
`go:embed`, SQLite uses the pure-Go driver (no CGO), so `CGO_ENABLED=0`
cross-compilation works for linux/amd64, linux/arm64, darwin, and windows.

### Tech Stack

| Layer    | Technology |
|----------|-----------|
| Backend  | Go, Gin, GORM, `glebarez/sqlite` (pure Go), gopsutil/v4, JWT (golang-jwt/v5), bcrypt, robfig/cron/v3 |
| Frontend | Vue 3 (single-file SPA), ECharts, xterm.js (Web terminal) |
| Storage  | SQLite |
| Delivery | single binary (`go:embed` frontend) |

---

## 2. Architecture

The backend follows a clean layered architecture:

```
HTTP request
   │
   ▼
router (gin.Engine, route registration + middleware wiring)
   │
   ▼
middleware (auth → RBAC perm → audit → access log)
   │
   ▼
api/v1 handlers (parse/validate request, call service, write response)
   │
   ▼
service (business logic, orchestration, external systems)
   │
   ▼
repository (GORM data access)
   │
   ▼
model (structs mapped to SQLite tables)
```

Rules:

- **handlers** do request parsing/validation and response shaping only — no
  business logic.
- **services** contain business logic and orchestrate repositories + system
  resources (gopsutil, Docker SDK, crontab, pty).
- **repositories** wrap GORM and are the only layer that touches the database.
- **models** are GORM structs with table mapping and JSON tags.

### Request Flow (authed panel API)

```
browser ──► /api/v1/xxx
             │  authMw      (parse JWT, inject user id/role)
             │  permMw      (RBAC permission check, optional per-route)
             │  auditMw     (record audit log)
             │  accessLogMw (record access log)
             ▼
          handler → service → repository → SQLite
```

The Agent API (`/agent/v1/*`) uses a **separate Agent Token** authentication
(not user JWT), guarded by `middleware.AgentAuth`.

---

## 3. Directory Structure

```
opsmini/
├── cmd/
│   └── agent/main.go        # entry point: parse flags, load config, init DB, start HTTP
├── configs/
│   └── config.yaml          # default configuration template
├── internal/
│   ├── config/              # config loading (Config struct + YAML + defaults)
│   ├── router/              # gin router, route registration, middleware wiring
│   ├── middleware/          # auth, perm (RBAC), audit, accesslog, cors, ratelimit, metrics_auth, agent
│   ├── api/v1/              # HTTP handlers (panel + agent endpoints)
│   ├── service/             # business logic (one file per module)
│   ├── repository/          # GORM data access (one file per model)
│   ├── model/               # GORM models + RBAC permission groups
│   └── pkg/
│       ├── jwt/             # JWT signing/verification (access + refresh)
│       ├── response/        # unified API response envelope
│       └── store/           # SQLite init, auto-migration, seed data
├── web/
│   ├── index.html           # single-file Vue3 SPA (7-language i18n, themes)
│   ├── embed.go             # go:embed the frontend into the binary
│   └── static/              # offline frontend dependencies
├── install/                 # install script
├── docs/                    # project documentation
└── Makefile                 # build / cross-compile / version targets
```

---

## 4. Module Reference

### 4.1 `cmd/agent/main.go`

Entry point. Responsibilities:

- parse flags (`-config`, `-version`);
- load config via `internal/config`;
- open SQLite via `internal/pkg/store`;
- seed built-in roles and the default admin account;
- construct services + handlers and hand them to `internal/router`;
- start the HTTP server (and the optional metrics endpoint).

### 4.2 `internal/config`

`config.go` defines the `Config` struct and loads YAML from the `-config` path.
When the file is missing, built-in defaults are returned. Sections:
`server`, `database`, `jwt`, `ai`, `agent`.

### 4.3 `internal/router`

`router.go` is the single place where every route is registered:

- public routes (`/healthz`, `/auth/login`, `/auth/refresh`, ...);
- authed panel routes (`/api/v1/*`) behind `authMw + audit + accesslog`;
- write routes additionally guarded by `permMw("permission.key")`;
- agent routes (`/agent/v1/*`) behind `middleware.AgentAuth`;
- WebSocket routes (`/terminal`, `/containers/:id/exec`).

**Convention:** every write endpoint (POST/PUT/DELETE) must carry a
`permMw(...)` guard. Read endpoints are open to any logged-in user unless they
expose sensitive data.

### 4.4 `internal/middleware`

| File | Purpose |
|------|---------|
| `auth.go` | JWT authentication; injects user id/role into context |
| `perm.go` | RBAC permission check (`permMw`) |
| `audit.go` | writes audit log entries |
| `accesslog.go` | writes access log entries |
| `cors.go` | CORS headers |
| `ratelimit.go` | login rate limiting |
| `metrics_auth.go` | bearer-token guard for `/metrics` |
| `agent.go` | Agent Token authentication for `/agent/v1` |

### 4.5 `internal/api/v1`

One handler file per module. Each handler:

1. binds/validates the request;
2. calls the corresponding service method;
3. returns a unified `response.OK` / `response.Error`.

Panel handlers live in the `v1` package (e.g. `system.go`, `file.go`,
`skill.go`); agent handlers are in `agent.go`.

### 4.6 `internal/service`

Business logic layer. One file per module. Notable modules:

| File | Module |
|------|--------|
| `auth.go`, `user.go`, `role.go`, `totp.go` | auth, users, RBAC, 2FA |
| `system.go`, `metrics.go`, `prometheus.go` | host info, metrics, prometheus export |
| `website.go`, `database.go`, `cron.go`, `crontab.go` | resource management |
| `file.go` | file operations with path-traversal protection |
| `docker.go` | Docker containers/images/volumes/networks |
| `ai.go` | LLM adapter (chat, streaming) |
| `skill.go`, `mcp.go` | AI skills + MCP servers |
| `alert.go`, `alertmonitor.go` | alert rules + evaluation |
| `security.go`, `securitymonitor.go`, `baseline.go`, `fim.go`, `threat.go`, `firewall.go`, `loginsecurity.go` | host security suite |
| `notification.go`, `audit.go`, `setting.go` | notifications, audit, settings |
| `appstore.go`, `apptemplate.go`, `appcategory.go` | app store / templates |

### 4.7 `internal/repository`

GORM data access, one file per model. Provides CRUD and query helpers. Never
contains business logic.

### 4.8 `internal/model`

GORM structs mapped to SQLite tables, plus `role.go` which is the single
authoritative source of RBAC permission groups (`PermGroups()`, `AllPermKeys()`,
`BuiltinRoles()`).

### 4.9 `internal/pkg`

| Package | Purpose |
|---------|---------|
| `jwt` | access/refresh token signing & verification |
| `response` | unified response envelope `{code,message,data}` |
| `store` | SQLite open, auto-migration, seed (default admin) |

### 4.10 `web`

- `index.html` — single-file Vue3 SPA: 7-language i18n, theme system, login,
  dashboard, monitoring, security, files, terminal, AI assistant, settings.
- `embed.go` — `go:embed` the frontend into the binary.
- `static/` — offline frontend dependencies (Vue, ECharts).

---

## 5. API Design

### 5.1 Response envelope

Every endpoint returns:

```json
{ "code": 0, "message": "ok", "data": { } }
```

- `code == 0` → success, `data` holds the payload;
- `code != 0` → business error, `message` describes it.

### 5.2 Authentication

- Panel API (`/api/v1`): JWT access token (`Authorization: Bearer <token>`),
  short-lived, refreshed via `/auth/refresh`.
- Agent API (`/agent/v1`): static Agent Token (config `agent.token`).

### 5.3 Endpoint families

| Family | Audience | Auth |
|--------|----------|------|
| `/api/v1/*` | browser UI | user JWT + RBAC |
| `/agent/v1/*` | external systems / proxy | Agent Token |

---

## 6. Data Model

Models are GORM structs in `internal/model`. Tables are auto-migrated at
startup by `internal/pkg/store`. Representative models:

- `User` (id, username, password hash, role, MFA secret, ...)
- `Role` (name, label, perms, builtin)
- `Website`, `Database`, `CronJob`
- `AlertRule`, `AlertEvent`, `Notification`
- `McpServer`, `Skill` (skill stored as directories on disk, not DB)
- `AuditLog`, `AccessLog`, `Setting`
- security: `BaselineResult`, `FimBaseline`, `FimChange`, `ThreatFinding`

---

## 7. RBAC & Permissions

Permission groups are defined in `internal/model/role.go` (`PermGroups()`), the
single source of truth. Three built-in roles:

- **admin** — perms `"*"` (all);
- **operator** — all perms except `user.*` and `settings.edit`;
- **readonly** — only `*.view` perms.

Frontend menus/buttons gate on the same keys via `hasPerm('key')`; the backend
enforces them via `permMw("key")`. When adding a new write feature you MUST do
all three:

1. add the permission key to `PermGroups()`;
2. guard the route with `permMw(...)`;
3. guard the button with `hasPerm(...)`.

---

## 8. Build & Deploy

```bash
make build          # build for current platform
make build-all      # cross-compile all platforms
make version        # print version info
```

The binary is static (no CGO). See [`build-and-deploy.md`](build-and-deploy.md)
for systemd, reverse proxy and upgrade instructions.

---

## 9. Development Guide

### 9.1 Adding a new module

Follow the layered pattern — create (or extend):

1. `internal/model/xxx.go` — GORM struct;
2. `internal/repository/xxx.go` — data access;
3. `internal/service/xxx.go` — business logic;
4. `internal/api/v1/xxx.go` — handler;
5. register routes in `internal/router/router.go`.

### 9.2 Adding a new write endpoint

1. add permission key to `internal/model/role.go`;
2. register route with `permMw("...")`;
3. add `hasPerm("...")` guard to the frontend button;
4. add i18n keys (all 7 languages) in `web/index.html`.

### 9.3 i18n convention

Frontend i18n dictionaries (`I18N`...`I18N8`) hold 7 languages:
`zh-CN`, `zh-TW`, `en`, `ja`, `ko`, `th`, `de`. Every new key must be added to
**all 7 languages** — the `t(key)` helper falls back to `zh-CN`, but a missing
translation shows Chinese to non-Chinese users.

### 9.4 Code style

- Exported Go functions/types carry a doc comment starting with their name.
- Comments are written in English.
- Every directory has a `README.md` describing its files.
