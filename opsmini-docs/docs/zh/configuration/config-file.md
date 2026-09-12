# 主配置文件

OpsMini 使用 YAML 配置文件。官方一键安装后默认位于 `/data/opsmini/config.yaml`；从源码运行时默认 `configs/config.yaml`，均可用 `-config` 参数指定。配置文件不存在时，程序使用内置默认值启动。

官方随包提供的示例已精简为**最小可运行配置**，其余运行时配置（AI、Agent 令牌、监控认证等）已迁移到面板「设置」页。

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
  level: error               # 日志级别：info / warn / error
  path: ""                   # 日志文件路径，留空则只输出到 stdout（systemd journal）
```

## 配置项说明

### server

| 参数 | 说明 | 默认 |
|------|------|------|
| `host` | 监听地址，`0.0.0.0` 表示所有网卡 | `0.0.0.0` |
| `port` | 监听端口 | `8888` |
| `secret_entry` | 安全入口路径前缀，如 `/opsmini_panel`，留空不启用 | 空 |

配置 `secret_entry` 后，面板与所有 API 都挂在前缀路径下（如 `http://<host>:8888/opsmini_panel`），可配合反向代理隐藏真实入口，防端口扫描与暴力探测。

### database

| 参数 | 说明 | 默认 |
|------|------|------|
| `path` | SQLite 数据文件路径 | `opsmini.db` |

### jwt

| 参数 | 说明 | 默认 |
|------|------|------|
| `access_ttl_seconds` | access token 有效期（秒） | `900` |
| `refresh_ttl_seconds` | refresh token 有效期（秒） | `604800` |

> JWT 签名密钥**不再**在此配置。首次启动时自动生成 32 字节随机密钥并持久化到数据库，不落配置文件、也不在界面展示，无需手动维护。

### log

| 参数 | 说明 | 默认 |
|------|------|------|
| `level` | 日志级别：`info` / `warn` / `error` | `error` |
| `path` | 日志文件路径，留空则只输出到 stdout（systemd journal） | 空 |

日志级别控制输出详细程度：`error` 仅输出错误（生产推荐，避免 SQL 查询日志刷屏）；`warn` 额外输出慢查询与警告；`info` 输出全部日志（含 SQL 查询，适合排查问题）。一键安装默认写入 `/data/opsmini/opsmini.log`。

## 可选配置段

以下字段在配置结构中仍受支持，但官方示例已省略（使用内置默认值），可按需显式声明。

### agent（命令白名单）

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

| 参数 | 说明 | 默认 |
|------|------|------|
| `allowed_commands` | 允许 `/agent/v1/commands` 执行的命令前缀白名单 | 见上 |

> `agent.token` 已废弃。Agent API 的认证令牌改由面板「设置 → API Token」页生成与管理，留空即禁用 Agent API。

### metrics（Prometheus 指标）

```yaml
metrics:
  enabled: true     # 是否启用 /metrics 端点
  user: ""          # Basic 认证用户名，空则无认证
  password: ""      # Basic 认证密码
```

| 参数 | 说明 | 默认 |
|------|------|------|
| `enabled` | 是否启用 `/metrics` 端点 | `true` |
| `user` | HTTP Basic 认证用户名，空则无认证 | 空 |
| `password` | HTTP Basic 认证密码 | 空 |

> 认证凭据优先级：面板「设置 → 监控导出」中的用户名/密码 **优先于** 这里的 `user`/`password`。用户名留空即开放访问。

## 已迁移到面板设置的配置

以下配置项已从 `config.yaml` 迁出，统一在面板「设置」页管理（存储于 SQLite，运行时即时生效）：

| 原配置 | 现管理位置 | 说明 |
|--------|-----------|------|
| `jwt.secret` | 自动生成（无需管理） | 首次启动生成随机密钥存库 |
| `ai.*` | 设置 → AI 大模型接入 | 模型 / API Key / Base URL / 启用开关 |
| `agent.token` | 设置 → API Token | Agent API 访问令牌，留空禁用 |
| `metrics.user/password` | 设置 → 监控导出 | 可运行时覆盖配置文件值 |

详见 [面板设置](panel-settings.md)。

## 环境变量

| 变量 | 说明 |
|------|------|
| `OPSMINI_AI_KEY` | AI API Key，**优先级最高**（高于面板设置与配置文件，避免密钥落盘） |

## 命令行参数

| 参数 | 说明 |
|------|------|
| `-config <path>` | 指定配置文件路径（一键安装默认 `/data/opsmini/config.yaml`，源码运行默认 `configs/config.yaml`） |
| `-version` | 打印版本信息并退出 |
| `-reset-mfa <username>` | 重置指定用户的 MFA 绑定（丢失验证码时使用），完成后退出 |
| `-reset-pass <username>` | 重置指定用户密码为随机强密码并打印，完成后退出 |

> `-reset-mfa` / `-reset-pass` 直接操作数据库，不启动服务，用于账号丢失恢复。
