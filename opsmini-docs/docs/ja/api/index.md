# REST API

OpsMini は外部に 2 セットの標準 REST API を公開し、ブラウザ UI と外部システムの連携呼び出しに供します。

## API 概要

| API | プレフィックス | 利用者 | 認証 |
|-----|------|--------|------|
| パネル API | `/api/v1` | ブラウザ（人） | ユーザーセッション JWT + RBAC |
| Agent API | `/agent/v1` | 外部システム（マシン） | Agent Token + コマンドホワイトリスト |
| Prometheus 指標 | `/metrics` | 監視システム | 任意の Basic 認証 |

統一レスポンス形式：`{ "code": 0, "message": "ok", "data": ... }`。`code` が 0 以外の場合はビジネスエラーです。

<div class="grid cards" markdown>

-   :material-web: **[パネル API（/api/v1）](browser-api.md)**

    ---

    認証、ユーザー、監視、ウェブサイト、コンテナ、ファイル、AI など全 UI 機能。

-   :material-api: **[Agent API（/agent/v1）](agent-api.md)**

    ---

    マシン対マシンのリソース管理とコマンド実行インターフェース。

-   :material-chart-line: **[Prometheus /metrics](prometheus.md)**

    ---

    node_exporter 互換の指標。Prometheus が直接 scrape できます。

</div>
