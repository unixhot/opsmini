# 架构设计

OpsMini 的技术架构与设计决策。

<div class="grid cards" markdown>

-   :material-sitemap-outline: **[总体架构](index.md)**

    ---

    单机面板、单体单二进制、前后端分离 + 嵌入打包。

-   :material-cube-outline: **[技术栈](tech-stack.md)**

    ---

    Go · Gin · GORM · SQLite（纯 Go）· Vue 3 · ECharts 选型理由。

-   :material-folder-outline: **[目录结构](directory.md)**

    ---

    分层架构（Controller → Service → Repository）与目录组织。

-   :material-server-network: **[后端架构设计](backend.md)**

    ---

    模块划分、分层与目录、数据模型、API 设计、关键流程与部署。

-   :material-robot-outline: **[AI 智能体设计](ai-agent.md)**

    ---

    自研 ReAct 内核、29 个工具与分级授权、上下文与预算控制。

-   :material-shield-lock-outline: **[主机安全设计](host-security.md)**

    ---

    评分模型、五项检测的判据与阈值、防锁死设计与边界。

</div>

## 核心架构决策

- **单机单体**：不做微服务，单进程单二进制交付
- **SQLite**：嵌入式零运维，业务数据量小完全够用
- **双 API**：`/api/v1` 面向 UI，`/mcp` 面向集成，认证与权限隔离
- **纯 Go 无 CGO**：任意平台一键交叉编译静态链接二进制
