<div align="center">

# OpsMini Backend Architecture

**后端架构设计**

[English](backend-architecture.md) · [简体中文](backend-architecture.zh-CN.md) · [繁體中文](backend-architecture.zh-TW.md) · [日本語](backend-architecture.ja.md) · [한국어](backend-architecture.ko.md) · [ไทย](backend-architecture.th.md) · [Deutsch](backend-architecture.de.md)

</div>

---

> 版本：v1.0  
> 依据：`web/index.html` UI 原型（登录/用户/仪表盘/监控/应用/容器/系统/文件/终端/计划任务/日志/AI 助手/面板设置/7 语言 i18n）

---

## 1. 概述

OpsMini 是一款**轻量级单机运维面板**（对标宝塔 / 1Panel），两大差异化：

1. **内置 AI 大模型**做系统管理（自然语言诊断 / 执行 / 日志分析）；
2. **对外暴露标准 REST API**，可被外部系统（监控平台、自动化脚本、第三方编排工具）集成调用。

> v1.0 范围说明：本版本**不包含 Proxy 中心节点**。每台主机上的 OpsMini 面板独立工作，
> 通过标准 REST API 对外提供能力；多机统一管理（Proxy）留待后续版本评估。

### 1.1 设计目标

| 目标 | 说明 |
|------|------|
| 轻量 | 单二进制部署，内存占用低，适合 1C1G 小主机 |
| 单机优先 | 核心场景是单台服务器，不引入分布式复杂度 |
| 可集成 | 通过标准 REST API 暴露能力，供外部系统集成调用 |
| 安全 | RBAC、2FA、安全入口、最小权限、密钥加密存储 |
| 可维护 | 模块化分层，Controller → Service → Repository 清晰 |

### 1.2 技术栈选型

| 层 | 选型 | 备选 | 理由 |
|----|------|------|------|
| 语言 | Go 1.22+ | — | 单二进制、交叉编译、并发好、生态成熟（1Panel 同栈） |
| Web 框架 | **Gin** | Echo / chi | 生态最大、中间件丰富、1Panel 同款 |
| 数据库 | **SQLite** | — | 嵌入式、零运维，贴合单机场景 |
| SQLite 驱动 | **modernc.org/sqlite** | mattn/go-sqlite3 | 纯 Go 无 CGO，交叉编译省心 |
| ORM | **GORM** | sqlx | 开发效率高；复杂查询可回落原生 SQL |
| 实时通信 | **gorilla/websocket** | — | 终端、日志 tail、指标推送 |
| 任务调度 | **robfig/cron** | — | 面板计划任务 |
| 系统监控 | **gopsutil** | 读 /proc | 跨平台 CPU/内存/磁盘/进程 |
| Docker | 官方 SDK | — | 容器/镜像/卷/网络 |
| 日志 | zerolog | zap | 轻量、结构化、低分配 |
| 认证 | JWT + refresh | session | 无状态 API + 可选会话 |
| 2FA | TOTP（RFC 6238） | — | 复用 pquerna/otp |
| AI | 抽象 LLM 接口 | — | OpenAI/DeepSeek/Qwen/Ollama 统一适配 |

### 1.3 总体架构

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

## 2. 核心架构决策

### 2.1 单体单二进制，不做微服务

**决策**：单进程单体。前端 Vue3 构建产物通过 `go:embed` 嵌入二进制，最终交付一个 `opsmini` 可执行文件。

**理由**：
- 运维面板的核心诉求是「装一台、管一台」，单体最贴合；
- 对外集成通过标准 REST API 完成，与单体不冲突；
- 微服务会带来部署、服务发现、分布式事务等无谓复杂度。

### 2.2 前后端分离 + 嵌入打包

- 开发期：Vue3 dev server 代理到 Gin（跨域 / 反代）；
- 生产期：`web/dist` 产物 `go:embed` 进二进制，单文件部署。

### 2.3 SQLite 而非 MySQL/Postgres

- 业务数据量小（配置、任务、用户、日志），SQLite 完全够用；
- 零运维、单文件、易备份（直接拷贝 .db）；
- 若未来面板自身数据需要高并发，可抽象 Repository 接口切换。

### 2.4 对外暴露标准 REST API

- v1.0 **不开发 Proxy 中心节点**，仅把面板能力暴露为标准 REST API；
- 两套 API 并存：`/api/v1`（面向浏览器 UI）与 `/agent/v1`（面向机器/第三方集成）；
- Agent API 独立认证（Agent Token）+ 命令白名单，与用户会话 JWT 隔离；
- 未来若需多机统一管理，可在 Agent API 之上加一层 Proxy 拉取调度（不在 v1.0 范围）。

---

## 3. 模块划分（对应 UI 原型）

| 后端模块 | 职责 | 对应原型页面 |
|----------|------|-------------|
| `auth` | 登录/退出、会话、2FA、RBAC | 登录页、用户管理 |
| `setting` | 面板配置、主题、菜单显隐、语言 | 面板设置（基础/外观/菜单） |
| `dashboard` | 指标聚合、实时推送 | 仪表盘 |
| `monitor` | 时序采集、告警规则、告警触发 | 监控 |
| `website` | Nginx 站点、域名、SSL 证书 | 应用管理-网站 |
| `database` | MySQL/PostgreSQL 实例与库 | 应用管理-数据库 |
| `store` | 软件安装/卸载/升级 | 应用管理-软件商店 |
| `container` | Docker 容器/镜像/卷/网络 | 容器管理 |
| `system` | 进程/网络/端口/磁盘 | 系统管理 |
| `file` | 文件浏览/上传/编辑/权限 | 文件 |
| `terminal` | Web SSH | 终端 |
| `cron` | 计划任务（面板+系统+用户 crontab） | 计划任务 |
| `log` | 日志收集/聚合/tail | 日志 |
| `ai` | LLM 接入、上下文、NL 执行 | AI 助手、面板设置-AI |
| `agent` | 对外 REST API（`/agent/v1`）、Agent Token 认证、命令白名单 | 面板设置-Proxy 接入 |

---

## 4. 分层与目录结构

### 4.1 分层

每个模块内部严格三层：

```
Controller（HTTP handler）：参数解析校验、调用 Service、组装响应
    → Service（业务逻辑）：编排 Repository 与系统资源、事务边界
    → Repository（GORM）：纯数据访问，不写业务
```

### 4.2 目录结构

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

## 5. 数据模型（SQLite Schema）

> GORM 迁移；敏感字段（密钥/Token）用 AES-GCM 加密后落库。

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

## 6. API 设计（RESTful，`/api/v1`）

统一响应：`{ "code": 0, "message": "ok", "data": ... }`，`code` 非 0 为业务错误。

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/auth/login` | 登录（返回 access + refresh） |
| POST | `/auth/refresh` | 刷新 token |
| POST | `/auth/logout` | 退出 |
| GET | `/auth/2fa/qrcode` | 生成 2FA 二维码 |
| POST | `/auth/2fa/verify` | 验证 2FA |
| GET | `/users` / POST / PUT / DELETE | 用户 CRUD |
| GET | `/dashboard/metrics` | 仪表盘指标 |
| GET | `/monitor/series?range=1h` | 时序数据 |
| CRUD | `/alert-rules` | 告警规则 |
| CRUD | `/websites` | 网站 |
| POST | `/websites/:id/ssl` | 签发/续期 SSL |
| CRUD | `/databases` | 数据库 |
| GET | `/store/apps` / POST `/store/apps/:id/install` | 软件商店 |
| GET | `/containers` / `/images` / `/volumes` / `/networks` | 容器四类 |
| POST | `/containers` 等 | 新建容器/拉镜像/建卷/建网络 |
| GET | `/system/processes` `/networks` `/ports` `/disks` | 系统资源 |
| GET/POST | `/files` / `/files/list` / `/files/upload` / `/files/edit` | 文件 |
| WS  | `/terminal/ws?cols=&rows=` | Web SSH |
| CRUD | `/cron-jobs` | 计划任务（面板类） |
| GET | `/cron-jobs/system` `/cron-jobs/user` | 系统/用户 crontab 只读 |
| GET | `/logs` | 日志列表 |
| POST | `/ai/chat` | AI 对话（流式 SSE） |
| POST | `/ai/execute` | NL 转操作（带权限确认） |
| GET/PUT | `/agent/config` | Agent API 配置（token、命令白名单） |
| GET/PUT | `/settings` | 面板设置 |
| GET | `/i18n/{lang}` | 语言包（前端也可内联） |
| GET | `/metrics` | **Prometheus 指标**（node_exporter 兼容，无 `/api/v1` 前缀） |

### 6.1 Prometheus 监控集成

OpsMini 内置 **node_exporter 兼容的 `/metrics` 端点**，无需额外安装 node_exporter，Prometheus 可直接 scrape：

```
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
```

已对齐 node_exporter 的核心指标（可直接套用社区 Node Dashboard）：

| 指标族 | 说明 |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | 每核各模式累计秒数 |
| `node_memory_MemTotal_bytes` 等 | 内存 Total/Free/Available/Buffers/Cached |
| `node_filesystem_size_bytes{mountpoint}` | 各挂载点容量/可用/使用率 |
| `node_network_receive_bytes_total{device}` | 各网卡收发流量 |
| `node_load1` / `node_load5` / `node_load15` | 负载 |
| `node_uname_info` / `node_boot_time_seconds` | 主机信息与启动时间 |

> 实现方式：复用 `gopsutil`（SystemService 已采集的数据），按 Prometheus 文本格式输出，
> 保持单二进制交付，不嵌入 node_exporter 进程。

### 6.2 Agent API（对外的标准 REST 接口）

面板除了 UI 使用的 `/api/v1`，还暴露一组**独立的机器对机器 REST API**（`/agent/v1`），供外部系统（监控平台、自动化脚本、第三方编排工具）集成调用。与面板 API 的区别：

- **认证**：用 Agent Token，而非用户会话 JWT；
- **范围**：聚焦资源管理与命令执行，不含 UI 专属能力（i18n/主题/菜单）与终端 WS；
- **风格**：面向自动化——幂等、可重试、统一 JSON 响应。

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/agent/v1/health` | 健康检查（探活） |
| GET | `/agent/v1/status` | 状态摘要：cpu/mem/磁盘/在线服务 |
| GET | `/agent/v1/system/info` | 主机信息（hostname/os/内核） |
| GET | `/agent/v1/system/processes` | 进程列表 |
| GET | `/agent/v1/system/ports` | 端口监听 |
| GET | `/agent/v1/system/disks` | 磁盘/挂载点 |
| GET | `/agent/v1/websites` / POST | 网站查询/创建 |
| GET | `/agent/v1/databases` | 数据库列表 |
| GET | `/agent/v1/containers` | 容器列表 |
| GET | `/agent/v1/cron-jobs` | 计划任务 |
| POST | `/agent/v1/commands` | 执行命令（白名单，返回执行结果） |

---

## 7. 对外 REST API 设计（差异化重点）

### 7.1 定位

v1.0 的 OpsMini 是**单机面板**，通过标准 REST API 对外暴露能力，供外部系统集成：

- **`/api/v1`（面板 API）**：面向浏览器 UI，用户会话 JWT + RBAC；
- **`/agent/v1`（Agent API）**：面向机器/第三方集成，Agent Token 认证 + 命令白名单。

> 与 salt minion/master 的「出站长连接 push」不同，OpsMini 直接**暴露 REST 供外部拉取调用**，
> 更接近 Prometheus 拉 exporter / 云厂商 OpenAPI 的思路。是否引入 Proxy 中心做多机统一管理，留待后续版本评估（v1.0 不做）。

### 7.2 调用模型

```
         HTTPS REST 调用（Agent Token）
   ┌──────────┐  ─────────────────────▶  ┌─────────┐
   │ 外部系统  │                          │ OpsMini │
   │ (监控/脚本 │  ◀─────────────────────  │ (单机面板)│
   │ /编排工具) │       统一 JSON 响应      └─────────┘
   └──────────┘
```

- **无长连接**：全部走标准 REST，不维持 WebSocket / gRPC 长连接；
- **流式场景**（日志 tail、实时指标）：REST 分页轮询即可，无需 SSE；
- **幂等**：查询类 GET 天然幂等；写操作（命令执行、资源创建）返回明确结果。

### 7.3 关键流程

1. 面板启动 → 读 `agent_config`（token + 命令白名单）；
2. 外部系统携带 `Authorization: Bearer <token>` 调 `/agent/v1/*`；
3. 中间件校验 token（恒定时间比较）→ 命中白名单放行，否则 401；
4. 高危操作（命令执行）走命令白名单二次校验，全部记审计；
5. 返回统一 JSON：`{ code, message, data }`。

### 7.4 安全

- **认证**：面板配独立随机 Agent Token（加密存储），请求头 `Authorization: Bearer <token>`；
- **传输**：HTTPS；高安全场景可加 IP 白名单；
- **命令白名单**：`/commands` 仅允许显式授权命令，全部记审计；
- **最小暴露**：`/agent/v1` 与 `/api/v1` 分离，Agent API 不暴露 UI 能力与终端。

### 7.5 两套 API 的定位

| 维度 | 面板 API `/api/v1` | Agent API `/agent/v1` |
|------|-------------------|----------------------|
| 使用者 | 浏览器（人） | 外部系统（机器） |
| 认证 | 用户会话 JWT + RBAC | Agent Token |
| 范围 | 全部 UI 功能 + 终端 WS | 资源管理 + 命令执行（无 UI/终端） |
| 设计 | 面向交互 | 面向自动化（幂等、可重试） |

---

## 8. 安全设计

| 维度 | 方案 |
|------|------|
| 密码 | bcrypt 哈希 |
| 会话 | 短期 access JWT（15min）+ refresh token（可撤销） |
| 2FA | TOTP（RFC 6238），登录时可选二次验证 |
| 授权 | RBAC 三角色：admin（全部）/ operator（日常运维）/ readonly（只读） |
| 安全入口 | 访问面板需带 secret 路径（如 `/opsmini_panel`），防端口扫描 |
| 密钥存储 | 面板密钥（API Key、Agent Token）AES-GCM 加密落库 |
| 终端 | Web SSH 仅对 operator 及以上开放，会话记录审计 |
| 防爆破 | 登录失败限流 + 锁定 |
| 审计 | 关键操作写 `audit_logs` |
| CSRF/XSS | API 用 Bearer Token 无 Cookie；前端转义 |

---

## 9. 关键流程

### 9.1 登录

```
输入账密 → bcrypt 校验 → 若开启 2FA 则要求 TOTP
  → 签发 access + refresh → 前端存 refresh（httpOnly/localStorage）
  → 后续请求带 Bearer access → 过期用 refresh 换新
```

### 9.2 Web SSH 终端

```
前端 ws://host/api/v1/terminal/ws?token=...
  → 服务端校验 token + 角色
  → 启动 pty（github.com/creack/pty）→ 双向数据转发
  → 关闭时回收 pty、记录会话时长
```

### 9.3 计划任务执行

- **面板任务**：robfig/cron 常驻调度，写 `cron_jobs`；
- **系统/用户任务**：直接读写 `/etc/crontab`、`/etc/cron.d/`、`/var/spool/cron/<user>`（只读展示 + 受控编辑）。

### 9.4 AI 助手

```
用户输入 → Service 拼上下文（当前页面/模块 + 系统状态）
  → 调 LLM（provider 适配：OpenAI/DeepSeek/Qwen/Ollama）
  → 若模型判定为「执行意图」→ 生成结构化 action + 参数
  → 命中权限白名单 → 执行 → 回传结果
  → 未授权/高危 → 要求用户二次确认
```

### 9.5 指标采集

- 采集器：gopsutil 每 5s 采样 → 内存 ring buffer；
- 历史：降采样后落 SQLite（1min 粒度保留 7 天）；
- 推送：WebSocket 广播给仪表盘/监控页订阅者。

---

## 10. 监控与可观测性

- 结构化日志（zerolog），级别分级，面板内「日志」页可查看；
- 指标：面板自身 + 主机指标统一由 monitor 模块采集；
- 健康检查：`/api/healthz` 返回进程/DB/磁盘状态。

---

## 11. 部署

### 11.1 产物

- `opsmini` 单二进制（约 25~30MB，`-s -w` 压缩后），内置前端、SQLite、静态资源；
- 配置：`/etc/opsmini/config.yaml` 或环境变量；
- 默认端口 8888，数据目录 `/var/lib/opsmini/`（opsmini.db）。

### 11.1.1 多架构发布（x64 + arm64）

发布需覆盖 **Linux** 的 **x86_64（amd64）** 与 **ARM64（arm64）** 两套指令集，另编译 macOS 目标仅用于开发调试。

| 目标平台 | 适用场景 |
|----------|----------|
| `linux/amd64` | 主流 x64 服务器（Intel/AMD，云厂商通用机型） |
| `linux/arm64` | ARM 服务器（Graviton、树莓派、鲲鹏、飞腾等） |
| `darwin/arm64` | Apple Silicon 开发机（本机调试） |
| `darwin/amd64` | Intel Mac（开发调试） |

> OpsMini 面向 **Linux 主机**，仅发布 Linux 安装包；macOS 目标仅用于开发调试，不作为交付产物。

**关键前提**：数据层采用纯 Go 驱动 `glebarez/sqlite`（底层 `modernc.org/sqlite`），**无 CGO 依赖**。因此 `CGO_ENABLED=0` 即可在任一平台一键交叉编译出**静态链接**的二进制，无需为每个架构准备 C 交叉工具链。

**构建方式**（`Makefile` 已就绪）：

```bash
make build        # 当前平台
make build-all    # 全平台交叉编译 → dist/
```

产物命名：`opsmini-<version>-<os>-<arch>`，版本号经 `-ldflags -X main.version` 注入，运行时 `opsmini -version` 可查。

已实测通过：4 个目标平台全部编译成功，`file` 校验架构正确（ELF x86-64 / ELF aarch64 / Mach-O arm64 / Mach-O x86_64），且均为静态链接。

### 11.2 服务化

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

### 11.3 反向代理（可选）

Nginx 反代 80/443 → 8888，证书由面板内网站模块管理或外部负载均衡承担。

---

## 12. 开发路线（里程碑）

| 阶段 | 内容 | 交付 |
|------|------|------|
| M1 骨架 | Go 工程、Gin 路由、SQLite、GORM 迁移、登录/用户/RBAC | 可跑的最小面板 |
| M2 系统能力 | gopsutil 指标、进程/端口/磁盘/网络、仪表盘+监控 | 单机监控闭环 |
| M3 资源管理 | 网站(Nginx)、数据库、文件、终端(pty)、计划任务 | 对标 1Panel 核心 |
| M4 容器 | Docker SDK：容器/镜像/卷/网络 | 容器管理 |
| M5 AI | LLM 适配层、上下文、NL 执行、AI 助手 | 差异化能力 |
| M6 Agent API | Agent 暴露标准 REST API（`/agent/v1`）、token 认证、命令白名单 | 对外集成能力 |
| M7 打磨 | i18n、审计、限流、测试、文档 | 生产就绪 |

---

## 13. 待确认的决策点

1. **Agent API 认证强度**：默认 Bearer Token；是否需要上 mTLS 双向证书（安全更高、部署更重）？
2. **Agent API 是否需要 IP 白名单**：默认关闭，仅靠 Token；多机/公网场景是否加 IP 限制？
3. **流式数据**：日志 tail / 实时指标，默认 REST 分页轮询，是否可接受？（v1.0 不引入 SSE/长连接）
4. **多机统一管理（未来）**：若后续引入 Proxy 中心，是复用现有 `/agent/v1` 做拉取调度，还是另设协议？（v1.0 范围外，仅备忘）
