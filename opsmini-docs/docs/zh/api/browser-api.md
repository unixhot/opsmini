# 面板 API（/api/v1）

面板 API 面向浏览器 UI，使用用户会话 JWT + RBAC 认证。

## 认证

- 登录接口 `POST /auth/login` 返回 `access` + `refresh` token
- 后续请求携带 `Authorization: Bearer <access_token>`
- access 过期后用 refresh 换新
- 登录限流（防爆破）；开启 2FA 后登录需先 `POST /auth/mfa/verify` 校验动态码

## 统一响应

```json
{ "code": 0, "message": "ok", "data": { } }
```

`code` 非 0 为业务错误。

## 端点一览

### 认证与用户

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/auth/login` | 登录（限流） |
| POST | `/auth/refresh` | 刷新 token |
| POST | `/auth/logout` | 退出 |
| POST | `/auth/mfa/verify` | 登录时校验 MFA 动态码 |
| GET | `/auth/mfa/status` | 查询当前用户 MFA 绑定状态 |
| POST | `/auth/mfa/setup` | 生成 MFA 绑定二维码/密钥 |
| POST | `/auth/mfa/enable` | 校验并启用 MFA |
| POST | `/auth/mfa/disable` | 解绑 MFA |
| GET | `/profile` | 个人资料（昵称/头像/邮箱） |
| PUT | `/profile` | 修改个人资料 |
| POST | `/profile/password` | 修改密码 |
| GET/POST/PUT/DELETE | `/users` | 用户 CRUD |
| GET/POST/PUT/DELETE | `/roles` | 角色 CRUD |
| GET | `/roles/groups` | 权限分组 |
| GET | `/permissions` | 当前用户权限 |

### 面板设置

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/settings` | 读取全部非敏感设置（`settings.view`） |
| PUT | `/settings` | 批量更新设置（`settings.edit`） |

> 敏感项（`jwt_secret`、`metrics_pass`）不返回；写保护项（`jwt_secret`）不可覆盖。详见 [面板设置](../configuration/panel-settings.md)。

### 监控与告警

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/dashboard/overview` | 仪表盘概览 |
| GET | `/dashboard/metrics` | 仪表盘指标 |
| GET | `/system/monitor` | 监控摘要 |
| CRUD | `/alert-rules` | 告警规则 |
| GET | `/alert-events` | 告警事件 |

### 通知

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/notifications` | 通知列表 |
| GET | `/notifications/unread-count` | 未读数 |
| PUT | `/notifications/read-all` | 全部已读 |
| PUT | `/notifications/:id/read` | 标记已读 |
| DELETE | `/notifications/:id` / `/notifications` | 删除单条 / 清空 |

### 资源管理

| 方法 | 路径 | 说明 |
|------|------|------|
| CRUD | `/websites` | 网站 |
| CRUD | `/databases` | 数据库 |
| GET/POST | `/apps` `/app-categories` | 软件商店与分类 |
| GET/POST | `/containers` `/images` `/volumes` `/networks` | 容器四类 |
| GET/POST | `/files` | 文件 |
| CRUD | `/cron-jobs` | 计划任务 |
| GET | `/logs` `/logs/tail` | 日志 |

### 系统与安全

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/system/info` `/processes` `/ports` `/disks` `/network` `/users` `/groups` `/firewall` | 系统资源 |
| GET/POST | `/security/...` | 主机安全（基线 / FIM / 威胁 / 防火墙 / 登录安全） |
| GET | `/audit-logs` `/access-logs` | 审计 / 访问日志 |

### AI 与集成

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/ai/chat` | AI 对话（函数调用） |
| POST | `/ai/chat/stream` | AI 对话（流式 SSE） |
| GET/POST/PUT/DELETE | `/mcp` | MCP 配置 |
| GET/POST/DELETE | `/skills` 等 | AI 技能（SkillHub 检索/安装/上传） |

### 终端

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/terminal` | WebSocket 终端（query token 认证） |
| GET | `/containers/:id/exec` | 容器 WebSocket 终端 |

## 说明

- 所有写操作受 RBAC 权限点控制（如 `website.create`、`container.edit`、`settings.edit`）
- 关键操作写审计日志，登录态请求写访问日志
