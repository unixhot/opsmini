<div align="center">

# OpsMini Developer Guide

**开发者手册**

[English](developer-guide.md) · [简体中文](developer-guide.zh-CN.md) · [繁體中文](developer-guide.zh-TW.md) · [日本語](developer-guide.ja.md) · [한국어](developer-guide.ko.md) · [ไทย](developer-guide.th.md) · [Deutsch](developer-guide.de.md)

</div>

---

> 版本：v1.0.0 · 语言：简体中文 · [English](developer-guide.md)

本手册深入讲解 OpsMini 代码库：架构、目录结构、各模块职责、API 设计、数据模型、RBAC 权限体系，以及如何扩展。

---

## 1. 项目概述

OpsMini 是一款**轻量级单主机运维面板**（对标宝塔 / 1Panel），两大核心差异化：

1. **AI 大模型运维** — 通过 LLM 适配层，用自然语言管理系统（诊断、分析日志、执行命令）。
2. **标准 REST 契约** — 面板暴露 `/api/v1`（浏览器 UI 使用）与 `/agent/v1`（机器对机器），便于中心 Proxy 拉取指标并统一管理多台主机。

交付形态为**单一静态二进制**：Vue3 前端通过 `go:embed` 内嵌，SQLite 使用纯 Go 驱动（无 CGO），因此 `CGO_ENABLED=0` 可交叉编译到 linux/amd64、linux/arm64、darwin、windows。

### 技术栈

| 层 | 技术 |
|----|------|
| 后端 | Go、Gin、GORM、`glebarez/sqlite`（纯 Go）、gopsutil/v4、JWT（golang-jwt/v5）、bcrypt、robfig/cron/v3 |
| 前端 | Vue 3（单文件 SPA）、ECharts、xterm.js（Web 终端） |
| 存储 | SQLite |
| 交付 | 单二进制（`go:embed` 内嵌前端） |

---

## 2. 架构

后端采用清晰的分层架构：

```
HTTP 请求
   │
   ▼
router（gin.Engine，路由注册 + 中间件装配）
   │
   ▼
middleware（认证 → RBAC 权限 → 审计 → 访问日志）
   │
   ▼
api/v1 handler（解析/校验请求，调用 service，写响应）
   │
   ▼
service（业务逻辑、编排、外部系统调用）
   │
   ▼
repository（GORM 数据访问）
   │
   ▼
model（映射到 SQLite 表的结构体）
```

规则：

- **handler** 只做请求解析/校验与响应封装，不含业务逻辑。
- **service** 承载业务逻辑，编排 repository 与系统资源（gopsutil、Docker SDK、crontab、pty）。
- **repository** 封装 GORM，是唯一接触数据库的层。
- **model** 是带表映射和 JSON tag 的 GORM 结构体。

### 请求流程（面板 API）

```
浏览器 ──► /api/v1/xxx
             │  authMw      （解析 JWT，注入用户 id/role）
             │  permMw      （RBAC 权限校验，按路由可选）
             │  auditMw     （记录审计日志）
             │  accessLogMw （记录访问日志）
             ▼
          handler → service → repository → SQLite
```

Agent API（`/agent/v1/*`）使用**独立的 Agent Token** 认证（非用户 JWT），由 `middleware.AgentAuth` 守护。

---

## 3. 目录结构

```
opsmini/
├── cmd/
│   └── agent/main.go        # 入口：解析参数、加载配置、初始化 DB、启动 HTTP
├── configs/
│   └── config.yaml          # 默认配置模板
├── internal/
│   ├── config/              # 配置加载（Config 结构 + YAML + 默认值）
│   ├── router/              # gin 路由、路由注册、中间件装配
│   ├── middleware/          # auth、perm(RBAC)、audit、accesslog、cors、ratelimit、metrics_auth、agent
│   ├── api/v1/              # HTTP handler（面板 + agent 端点）
│   ├── service/             # 业务逻辑（每个模块一个文件）
│   ├── repository/          # GORM 数据访问（每个 model 一个文件）
│   ├── model/               # GORM 模型 + RBAC 权限分组
│   └── pkg/
│       ├── jwt/             # JWT 签发/校验（access + refresh）
│       ├── response/        # 统一 API 响应封装
│       └── store/           # SQLite 初始化、自动迁移、种子数据
├── web/
│   ├── index.html           # 单文件 Vue3 SPA（7 语言 i18n、主题系统）
│   ├── embed.go             # go:embed 将前端内嵌进二进制
│   └── static/              # 前端离线依赖
├── install/                 # 安装脚本
├── docs/                    # 项目文档
└── Makefile                 # 构建 / 交叉编译 / 版本目标
```

---

## 4. 模块说明

### 4.1 `cmd/agent/main.go`

入口。职责：

- 解析参数（`-config`、`-version`）；
- 通过 `internal/config` 加载配置；
- 通过 `internal/pkg/store` 打开 SQLite；
- 初始化内置角色和默认 admin 账号；
- 组装 service + handler 交给 `internal/router`；
- 启动 HTTP 服务（及可选的 metrics 端点）。

### 4.2 `internal/config`

`config.go` 定义 `Config` 结构，从 `-config` 路径加载 YAML；文件不存在时返回内置默认值。配置段：`server`、`database`、`jwt`、`ai`、`agent`。

### 4.3 `internal/router`

`router.go` 是所有路由注册的唯一入口：

- 公开路由（`/healthz`、`/auth/login`、`/auth/refresh` 等）；
- 认证面板路由（`/api/v1/*`），挂在 `authMw + audit + accesslog` 之后；
- 写路由额外加 `permMw("permission.key")` 守卫；
- agent 路由（`/agent/v1/*`），挂在 `middleware.AgentAuth` 之后；
- WebSocket 路由（`/terminal`、`/containers/:id/exec`）。

**约定：** 每个写端点（POST/PUT/DELETE）必须带 `permMw(...)` 守卫。读端点对任意登录用户开放（除非涉及敏感数据）。

### 4.4 `internal/middleware`

| 文件 | 作用 |
|------|------|
| `auth.go` | JWT 认证；向上下文注入用户 id/role |
| `perm.go` | RBAC 权限校验（`permMw`） |
| `audit.go` | 写审计日志 |
| `accesslog.go` | 写访问日志 |
| `cors.go` | CORS 头 |
| `ratelimit.go` | 登录限流 |
| `metrics_auth.go` | `/metrics` 的 Bearer Token 守卫 |
| `agent.go` | `/agent/v1` 的 Agent Token 认证 |

### 4.5 `internal/api/v1`

每个模块一个 handler 文件。每个 handler：

1. 绑定/校验请求；
2. 调用对应 service 方法；
3. 返回统一的 `response.OK` / `response.Error`。

面板 handler 位于 `v1` 包（如 `system.go`、`file.go`、`skill.go`）；agent handler 位于 `agent.go`。

### 4.6 `internal/service`

业务逻辑层，每个模块一个文件。主要模块：

| 文件 | 模块 |
|------|------|
| `auth.go`、`user.go`、`role.go`、`totp.go` | 认证、用户、RBAC、双因素验证 |
| `system.go`、`metrics.go`、`prometheus.go` | 主机信息、指标、prometheus 导出 |
| `website.go`、`database.go`、`cron.go`、`crontab.go` | 资源管理 |
| `file.go` | 文件操作（含目录穿越防护） |
| `docker.go` | Docker 容器/镜像/卷/网络 |
| `ai.go` | LLM 适配器（对话、流式） |
| `skill.go`、`mcp.go` | AI 技能 + MCP 服务 |
| `alert.go`、`alertmonitor.go` | 告警规则 + 评估 |
| `security.go`、`securitymonitor.go`、`baseline.go`、`fim.go`、`threat.go`、`firewall.go`、`loginsecurity.go` | 主机安全套件 |
| `notification.go`、`audit.go`、`setting.go` | 通知、审计、设置 |
| `appstore.go`、`apptemplate.go`、`appcategory.go` | 应用商店 / 模版 |

### 4.7 `internal/repository`

GORM 数据访问，每个 model 一个文件，提供 CRUD 和查询辅助。不含业务逻辑。

### 4.8 `internal/model`

映射到 SQLite 表的 GORM 结构体，其中 `role.go` 是 RBAC 权限分组的唯一权威来源（`PermGroups()`、`AllPermKeys()`、`BuiltinRoles()`）。

### 4.9 `internal/pkg`

| 包 | 作用 |
|----|------|
| `jwt` | access/refresh token 签发与校验 |
| `response` | 统一响应封装 `{code,message,data}` |
| `store` | SQLite 打开、自动迁移、种子数据（默认 admin） |

### 4.10 `web`

- `index.html` — 单文件 Vue3 SPA：7 语言 i18n、主题系统、登录、仪表盘、监控、安全、文件、终端、AI 助手、设置。
- `embed.go` — `go:embed` 将前端内嵌进二进制。
- `static/` — 前端离线依赖（Vue、ECharts）。

---

## 5. API 设计

### 5.1 响应封装

每个端点返回：

```json
{ "code": 0, "message": "ok", "data": { } }
```

- `code == 0` → 成功，`data` 承载数据；
- `code != 0` → 业务错误，`message` 描述错误。

### 5.2 认证

- 面板 API（`/api/v1`）：JWT access token（`Authorization: Bearer <token>`），短期有效，通过 `/auth/refresh` 刷新。
- Agent API（`/agent/v1`）：静态 Agent Token（配置 `agent.token`）。

### 5.3 端点族

| 族 | 受众 | 认证 |
|----|------|------|
| `/api/v1/*` | 浏览器 UI | 用户 JWT + RBAC |
| `/agent/v1/*` | 外部系统 / Proxy | Agent Token |

---

## 6. 数据模型

模型是 `internal/model` 里的 GORM 结构体，启动时由 `internal/pkg/store` 自动迁移建表。代表性模型：

- `User`（id、username、密码哈希、role、MFA 密钥等）
- `Role`（name、label、perms、builtin）
- `Website`、`Database`、`CronJob`
- `AlertRule`、`AlertEvent`、`Notification`
- `McpServer`、`Skill`（技能以磁盘目录存储，非 DB）
- `AuditLog`、`AccessLog`、`Setting`
- 安全：`BaselineResult`、`FimBaseline`、`FimChange`、`ThreatFinding`

---

## 7. RBAC 与权限

权限分组在 `internal/model/role.go`（`PermGroups()`）中定义，是唯一权威来源。三个内置角色：

- **admin** — 权限 `"*"`（全部）；
- **operator** — 除 `user.*` 和 `settings.edit` 外的所有权限；
- **readonly** — 仅 `*.view` 查看权限。

前端菜单/按钮用 `hasPerm('key')` 做同一套 key 的显隐控制；后端用 `permMw("key")` 强制校验。新增写功能时**必须**三处同时改：

1. 在 `PermGroups()` 增加权限 key；
2. 路由加 `permMw(...)` 守卫；
3. 前端按钮加 `hasPerm(...)` 守卫。

---

## 8. 构建与部署

```bash
make build          # 当前平台构建
make build-all      # 交叉编译所有平台
make version        # 打印版本信息
```

二进制为静态（无 CGO）。systemd、反向代理、升级说明见 [`build-and-deploy.md`](build-and-deploy.md)。

---

## 9. 开发指南

### 9.1 新增模块

遵循分层模式，创建（或扩展）：

1. `internal/model/xxx.go` — GORM 结构；
2. `internal/repository/xxx.go` — 数据访问；
3. `internal/service/xxx.go` — 业务逻辑；
4. `internal/api/v1/xxx.go` — handler；
5. 在 `internal/router/router.go` 注册路由。

### 9.2 新增写端点

1. 在 `internal/model/role.go` 增加权限 key；
2. 用 `permMw("...")` 注册路由；
3. 前端按钮加 `hasPerm("...")` 守卫；
4. 在 `web/index.html` 补 i18n key（7 语言全）。

### 9.3 i18n 规范

前端 i18n 字典（`I18N`...`I18N8`）含 7 语言：`zh-CN`、`zh-TW`、`en`、`ja`、`ko`、`th`、`de`。每个新 key 必须补全**全部 7 语言**——`t(key)` 会在缺失时回退到 `zh-CN`，但缺失翻译会让非中文用户看到中文。

### 9.4 代码风格

- 导出的 Go 函数/类型带以其名字开头的文档注释。
- 注释统一使用英文。
- 每个目录都有描述其文件的 `README.md`。
