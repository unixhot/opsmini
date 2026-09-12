# 面板设置

面板「设置」页集中管理运行时配置，修改即时生效并持久化到 SQLite 数据库，无需编辑配置文件或重启服务。部分此前写在 `config.yaml` 里的配置（AI、Agent 令牌、监控认证）已迁移到这里。

> 访问「面板设置」需要 `settings.view` 权限，保存修改需要 `settings.edit` 权限（默认 `admin` 角色具备）。

## 基础设置

| 项 | 说明 |
|----|------|
| 面板端口 | 面板监听端口（对应 `server.port`） |
| 面板域名 | 面板访问域名，用于生成链接与反向代理场景 |
| 安全入口 | 隐藏路径前缀（对应 `server.secret_entry`），如 `/opsmini_panel`，留空不启用 |
| 开启 HTTPS | 是否启用 HTTPS（推荐配合反向代理在边缘终止 TLS） |

## 二步验证（2FA）

- **全局开关**：开启后，所有已绑定验证器的账号登录时均需输入 6 位动态码
- 每个用户可独立绑定/解绑 TOTP 验证器（Google Authenticator / 1Password / 各云厂商验证器 APP 兼容）
- 丢失验证器时，可登录服务器用 `opsmini -reset-mfa <username>` 重置

详见 [用户与权限](../features/users-rbac.md#2fa)。

## AI 大模型接入 {: #ai-config }

统一在此配置内置 AI 助手的模型接入，替代原先的 `config.yaml` `ai` 段：

| 项 | 说明 |
|----|------|
| 启用 AI 助手 | 总开关，关闭后 AI 相关功能不可用 |
| 模型服务商 | `openai` / `deepseek` / `qwen` / `ollama` |
| 模型 | 模型名，如 `gpt-4o`、`deepseek-chat`、`qwen-plus` |
| API Key | 服务商密钥；留空可读环境变量 `OPSMINI_AI_KEY` |
| API 地址 | OpenAI 兼容接口 Base URL，如 `https://api.openai.com/v1` |
| 自然语言执行操作 | 是否允许 AI 执行运维操作 |
| 日志智能分析 | 是否启用 AI 日志分析 |
| 告警智能诊断 | 是否启用 AI 告警诊断 |

> 配置优先级：环境变量 `OPSMINI_AI_KEY` > 面板设置 > 配置文件残留的 `ai.*`。详见 [AI 助手](../features/ai-assistant.md)。

## API Token

为外部系统（监控平台、自动化脚本、第三方编排工具）生成访问令牌：

- 生成的令牌用于调用 `/agent/v1` 接口，请求头携带 `Authorization: Bearer <token>`
- 留空则**禁用** Agent API
- 支持「生成随机」一键生成强随机令牌；保存后生效
- 泄露后应立即重新生成

命令白名单仍在 `config.yaml` 的 `agent.allowed_commands` 中维护。详见 [Agent API](../api/agent-api.md)。

## 外观与主题

- **自定义主色**：设置面板品牌主色（企业品牌色 / 个人偏好），默认 OpsMini 蓝 `#4f6ef7`
- **菜单显示 / 语言**：面板界面语言（7 语言）与菜单可见性

## 监控导出（Prometheus）

控制 `/metrics` 端点（node_exporter 兼容）的认证：

| 项 | 说明 |
|----|------|
| 启用认证 | 是否为 `/metrics` 开启 HTTP Basic 认证 |
| 用户名 / 密码 | Basic 认证凭据，用户名留空即开放访问 |

> 此处设置优先于 `config.yaml` 的 `metrics.user/password`。Prometheus 抓取时需配置对应 `basic_auth`。详见 [Prometheus 指标](../api/prometheus.md)。

## 应用模版（软件商店）

管理软件商店的应用模版：

- **同步官方模版**：从 opsmini.com 官网应用商店一键同步官方应用模版
- **手动导入/导出**：离线环境可从官网下载模版后手动导入，也可导出本地模版备份
- **数据目录**：应用数据持久化目录（安装脚本通过 `DATA_DIR` 环境变量引用）

## 通知

配置告警通知渠道：

| 渠道 | 说明 |
|------|------|
| SMTP 邮件 | SMTP 服务器 / 端口 / 发件人 / 收件人 / 用户名 / 密码 |
| 企业微信 | 群机器人 Webhook 地址 |
| 钉钉 | 群机器人 Webhook 地址 |
| 飞书 | 群机器人 Webhook 地址 |

告警触发后按此处配置的渠道发送通知。

## 日志保留

- **面板日志保留时间**：审计日志与访问日志的保留天数（默认 7 天），过期自动清理
- 告警事件保留天数可另行设置（默认 30 天）

## 设置存储

面板设置以 key-value 形式存于 SQLite 的 `settings` 表，运行时读取。两类敏感项由服务端保护：

- **只读敏感**（`jwt_secret`、`metrics_pass`）：接口不返回给前端
- **写保护**（`jwt_secret`）：客户端不可覆盖，由服务端自动生成管理
