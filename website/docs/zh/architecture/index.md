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

</div>

## 核心架构决策

- **单机单体**：不做微服务，单进程单二进制交付
- **SQLite**：嵌入式零运维，业务数据量小完全够用
- **双 API**：`/api/v1` 面向 UI，`/agent/v1` 面向集成，认证与权限隔离
- **纯 Go 无 CGO**：任意平台一键交叉编译静态链接二进制
