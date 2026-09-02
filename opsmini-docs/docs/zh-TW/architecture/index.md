# 架構設計

OpsMini 的技術架構與設計決策。

<div class="grid cards" markdown>

-   :material-sitemap-outline: **[總體架構index.md**

    ---

    單機面板、單體單二進位制、前後端分離 + 嵌入打包。

-   :material-cube-outline: **[技術棧tech-stack.md**

    ---

    Go · Gin · GORM · SQLite（純 Go）· Vue 3 · ECharts 選型理由。

-   :material-folder-outline: **[目錄結構directory.md**

    ---

    分層架構（Controller → Service → Repository）與目錄組織。

</div>

## 核心架構決策

- **單機單體**：不做微服務，單程序單二進位制交付
- **SQLite**：嵌入式零運維，業務資料量小完全夠用
- **雙 API**：`/api/v1` 面向 UI，`/agent/v1` 面向整合，認證與許可權隔離
- **純 Go 無 CGO**：任意平臺一鍵交叉編譯靜態連結二進位制
