# ディレクトリ構成

## レイヤー構造

各モジュールの内部は厳密に 3 層です：

```
Controller（HTTP handler）：パラメータの解析・検証、Service の呼び出し、レスポンスの組み立て
    → Service（ビジネスロジック）：Repository とシステムリソースのオーケストレーション、トランザクション境界
    → Repository（GORM）：純粋なデータアクセス、ビジネスロジックは書かない
```

## ディレクトリ編成

```
opsmini/
├── cmd/
│   └── agent/main.go          # メインプログラムのエントリポイント
├── internal/
│   ├── router/                # ルーティング登録、API バージョン
│   ├── middleware/            # 認証、権限、監査、レート制限、セキュリティ入口
│   ├── api/v1/                # Controller（モジュール別パッケージ）
│   ├── service/               # Service（モジュール別パッケージ）
│   ├── repository/            # Repository（モジュール別パッケージ）
│   ├── model/                 # GORM データモデル
│   ├── config/                # 設定読み込み
│   └── pkg/                   # 汎用ツール（jwt/response/store）
├── web/                       # Vue3 フロントエンド（ビルド後に embed）
│   ├── index.html             # SPA エントリポイント
│   └── static/                # echarts/vue/xterm などの静的ライブラリ
├── configs/                   # デフォルト設定例
├── docs/                      # ドキュメント
└── install/                   # ワンクリックインストールスクリプト
```

## 主要モジュール

| モジュール | 責務 |
|------|------|
| `auth` | ログイン / ログアウト、セッション、2FA、RBAC |
| `setting` | パネル設定、テーマ、メニュー表示/非表示、言語 |
| `dashboard` / `monitor` | 指標集約、時系列収集、アラートルール |
| `website` / `database` / `appstore` | ウェブサイト、データベース、アプリストア |
| `container` | Docker コンテナ / イメージ / ボリューム / ネットワーク |
| `system` / `file` / `terminal` | システムリソース、ファイル、Web ターミナル |
| `cron` / `log` | スケジュールタスク、ログ |
| `ai` | LLM 接続、コンテキスト、NL 実行 |
| `security` | ベースライン、FIM、脅威、ファイアウォール、ログインセキュリティ |
| `agent` | 外部向け REST API（`/agent/v1`） |
