# Agent API（/agent/v1）

Agent API 是面向**机器 / 第三方系统**的标准 REST 接口，供监控平台、自动化脚本、编排工具集成调用。

## 与面板 API 的区别

| 维度 | 面板 API `/api/v1` | Agent API `/agent/v1` |
|------|-------------------|----------------------|
| 使用者 | 浏览器（人） | 外部系统（机器） |
| 认证 | 用户会话 JWT + RBAC | Agent Token（Bearer） |
| 范围 | 全部 UI 功能 + 终端 WS | 资源查询 + 命令执行（无 UI / 终端） |
| 设计 | 面向交互 | 面向自动化（幂等、可重试） |

## 认证

1. 在面板「设置 → API Token」页生成访问令牌（令牌持久化在数据库，不再写在 `config.yaml`）
2. 请求携带 `Authorization: Bearer <token>`
3. 中间件以恒定时间比较校验 token；令牌留空即禁用 Agent API

## 端点一览

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/agent/v1/health` | 健康检查（探活） |
| GET | `/agent/v1/version` | 版本信息 |
| GET | `/agent/v1/status` | 状态摘要：cpu / mem / 磁盘 / 在线服务 |
| GET | `/agent/v1/system/info` | 主机信息（hostname / os / 内核） |
| GET | `/agent/v1/system/processes` | 进程列表 |
| GET | `/agent/v1/system/ports` | 端口监听 |
| GET | `/agent/v1/system/disks` | 磁盘 / 挂载点 |
| GET | `/agent/v1/websites` | 网站列表 |
| GET | `/agent/v1/databases` | 数据库列表 |
| GET | `/agent/v1/cron-jobs` | 计划任务 |
| GET | `/agent/v1/containers` | 容器列表 |
| POST | `/agent/v1/commands` | 执行命令（白名单） |
| POST | `/agent/v1/script/run` | 运行脚本 |
| POST | `/agent/v1/file/upload` | 上传文件 |

## 命令执行与白名单

`/commands` 仅允许执行显式授权的命令前缀，全部记审计。白名单在 `config.yaml` 的 `agent.allowed_commands` 中维护：

```yaml
agent:
  allowed_commands:
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

## 调用示例

```bash
curl -H "Authorization: Bearer <token>" \
     https://<host>:8888/agent/v1/status
```

## 安全建议

- 使用 HTTPS 传输
- 高安全场景可加 IP 白名单限制来源
- 命令白名单最小化，仅授权必要命令
- 令牌泄露后立即在面板「设置 → API Token」重新生成
