# アーキテクチャ設計

OpsMini の技術アーキテクチャと設計上の意思決定。

<div class="grid cards" markdown>

-   :material-sitemap-outline: **[全体アーキテクチャ](index.md)**

    ---

    単一ホストパネル、モノリス単一バイナリ、フロントエンド/バックエンド分離 + 埋め込みパッケージング。

-   :material-cube-outline: **[技術スタック](tech-stack.md)**

    ---

    Go · Gin · GORM · SQLite（純 Go）· Vue 3 · ECharts の選定理由。

-   :material-folder-outline: **[ディレクトリ構成](directory.md)**

    ---

    レイヤードアーキテクチャ（Controller → Service → Repository）とディレクトリ編成。

</div>

## 主要なアーキテクチャ上の意思決定

- **単一ホスト・モノリス**：マイクロサービスにせず、単一プロセス・単一バイナリで配布
- **SQLite**：組み込みで運用ゼロ、業務データ量が小さいため十分
- **デュアル API**：`/api/v1` は UI 向け、`/agent/v1` は連携向け。認証と権限を分離
- **純 Go・CGO なし**：任意のプラットフォームでワンコマンドのクロスコンパイルにより静的リンクバイナリを生成
