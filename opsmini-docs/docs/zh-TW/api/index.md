# REST API

OpsMini 對外暴露兩套標準 REST API，供瀏覽器 UI 與外部系統整合呼叫。

## API 概覽

| API | 字首 | 使用者 | 認證 |
|-----|------|--------|------|
| 面板 API | `/api/v1` | 瀏覽器（人） | 使用者會話 JWT + RBAC |
| Agent API | `/agent/v1` | 外部系統（機器） | Agent Token + 命令白名單 |
| Prometheus 指標 | `/metrics` | 監控系統 | 可選 Basic 認證 |

統一響應格式：`{ "code": 0, "message": "ok", "data": ... }`，`code` 非 0 為業務錯誤。

<div class="grid cards" markdown>

-   :material-web: **[面板 API（/api/v1）browser-api.md**

    ---

    認證、使用者、監控、網站、容器、檔案、AI 等全部 UI 能力。

-   :material-api: **[Agent API（/agent/v1）agent-api.md**

    ---

    機器對機器的資源管理與命令執行介面。

-   :material-chart-line: **[Prometheus /metricsprometheus.md**

    ---

    node_exporter 相容指標，Prometheus 直接 scrape。

</div>
