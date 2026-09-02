# REST API

OpsMini 对外暴露两套标准 REST API，供浏览器 UI 与外部系统集成调用。

## API 概览

| API | 前缀 | 使用者 | 认证 |
|-----|------|--------|------|
| 面板 API | `/api/v1` | 浏览器（人） | 用户会话 JWT + RBAC |
| Agent API | `/agent/v1` | 外部系统（机器） | Agent Token + 命令白名单 |
| Prometheus 指标 | `/metrics` | 监控系统 | 可选 Basic 认证 |

统一响应格式：`{ "code": 0, "message": "ok", "data": ... }`，`code` 非 0 为业务错误。

<div class="grid cards" markdown>

-   :material-web: **[面板 API（/api/v1）](browser-api.md)**

    ---

    认证、用户、监控、网站、容器、文件、AI 等全部 UI 能力。

-   :material-api: **[Agent API（/agent/v1）](agent-api.md)**

    ---

    机器对机器的资源管理与命令执行接口。

-   :material-chart-line: **[Prometheus /metrics](prometheus.md)**

    ---

    node_exporter 兼容指标，Prometheus 直接 scrape。

</div>
