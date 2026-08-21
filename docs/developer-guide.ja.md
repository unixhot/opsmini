<div align="center">

# OpsMini Developer Guide

**開発者ガイド**

[English](developer-guide.md) · [简体中文](developer-guide.zh-CN.md) · [繁體中文](developer-guide.zh-TW.md) · [日本語](developer-guide.ja.md) · [한국어](developer-guide.ko.md) · [ไทย](developer-guide.th.md) · [Deutsch](developer-guide.de.md)

</div>

---

> バージョン：v1.0.0 · 言語：日本語 · [English](developer-guide.md)

本ガイドは OpsMini のコードベースを深く解説します：アーキテクチャ、ディレクトリ構成、各モジュール、API 設計、データモデル、RBAC、および拡張方法。

---

## 1. 概要

OpsMini は**軽量な単一ホストのサーバー管理パネル**（BaoTa / 1Panel に類似）であり、次の 2 点で差別化されています：

1. **AI による運用** — LLM アダプターにより、自然言語でシステムを管理できます（診断、ログ分析、コマンド実行）。
2. **クリーンな REST コントラクト** — パネルは `/api/v1`（ブラウザー UI 用）と `/agent/v1`（マシン間通信用）を公開し、中央プロキシがメトリクスを取得して多数のホストを管理できます。

**単一の静的バイナリ**として配布されます：Vue3 フロントエンドは `go:embed` で埋め込まれ、SQLite は純粋な Go ドライバー（CGO なし）を使用するため、`CGO_ENABLED=0` のクロスコンパイルが linux/amd64、linux/arm64、darwin、windows で動作します。

### 技術スタック

| レイヤー | テクノロジー |
|----------|-------------|
| バックエンド | Go、Gin、GORM、`glebarez/sqlite`（純粋な Go）、gopsutil/v4、JWT（golang-jwt/v5）、bcrypt、robfig/cron/v3 |
| フロントエンド | Vue 3（単一ファイル SPA）、ECharts、xterm.js（Web ターミナル） |
| ストレージ | SQLite |
| 配布 | 単一バイナリ（`go:embed` フロントエンド） |

---

## 2. アーキテクチャ

バックエンドは明確なレイヤー化アーキテクチャに従います：

```
HTTP リクエスト
   │
   ▼
router（gin.Engine、ルート登録 + ミドルウェア配線）
   │
   ▼
middleware（認証 → RBAC 権限 → 監査 → アクセスログ）
   │
   ▼
api/v1 handler（リクエストの解析/検証、service 呼び出し、レスポンス書き込み）
   │
   ▼
service（ビジネスロジック、オーケストレーション、外部システム）
   │
   ▼
repository（GORM データアクセス）
   │
   ▼
model（SQLite テーブルにマッピングされた構造体）
```

ルール：

- **handler** はリクエストの解析/検証とレスポンス整形のみを行い、ビジネスロジックは含みません。
- **service** はビジネスロジックを含み、repository とシステムリソース（gopsutil、Docker SDK、crontab、pty）をオーケストレーションします。
- **repository** は GORM をラップし、データベースにアクセスする唯一のレイヤーです。
- **model** はテーブルマッピングと JSON タグを持つ GORM 構造体です。

### リクエストフロー（認証済みパネル API）

```
ブラウザー ──► /api/v1/xxx
             │  authMw      （JWT を解析し、ユーザー id/role を注入）
             │  permMw      （RBAC 権限チェック、ルートごとに任意）
             │  auditMw     （監査ログを記録）
             │  accessLogMw （アクセスログを記録）
             ▼
          handler → service → repository → SQLite
```

Agent API（`/agent/v1/*`）は、ユーザー JWT ではなく**別個の Agent Token** 認証を使用し、`middleware.AgentAuth` によって保護されます。

---

## 3. ディレクトリ構造

```
opsmini/
├── cmd/
│   └── agent/main.go        # エントリポイント：フラグ解析、設定読み込み、DB 初期化、HTTP 起動
├── configs/
│   └── config.yaml          # デフォルト設定テンプレート
├── internal/
│   ├── config/              # 設定読み込み（Config 構造体 + YAML + デフォルト値）
│   ├── router/              # gin ルーター、ルート登録、ミドルウェア配線
│   ├── middleware/          # auth、perm（RBAC）、audit、accesslog、cors、ratelimit、metrics_auth、agent
│   ├── api/v1/              # HTTP ハンドラー（パネル + agent エンドポイント）
│   ├── service/             # ビジネスロジック（モジュールごとに 1 ファイル）
│   ├── repository/          # GORM データアクセス（モデルごとに 1 ファイル）
│   ├── model/               # GORM モデル + RBAC 権限グループ
│   └── pkg/
│       ├── jwt/             # JWT 署名/検証（access + refresh）
│       ├── response/        # 統一 API レスポンスエンベロープ
│       └── store/           # SQLite 初期化、自動マイグレーション、シードデータ
├── web/
│   ├── index.html           # 単一ファイル Vue3 SPA（7 言語 i18n、テーマ）
│   ├── embed.go             # go:embed によるフロントエンドのバイナリ埋め込み
│   └── static/              # オフラインのフロントエンド依存関係
├── install/                 # インストールスクリプト
├── docs/                    # プロジェクトドキュメント
└── Makefile                 # ビルド / クロスコンパイル / バージョンターゲット
```

---

## 4. モジュールリファレンス

### 4.1 `cmd/agent/main.go`

エントリポイント。責務：

- フラグの解析（`-config`、`-version`）；
- `internal/config` による設定読み込み；
- `internal/pkg/store` による SQLite のオープン；
- 組み込みロールとデフォルト admin アカウントのシード；
- service + handler を構築し、`internal/router` に渡す；
- HTTP サーバーの起動（およびオプションの metrics エンドポイント）。

### 4.2 `internal/config`

`config.go` は `Config` 構造体を定義し、`-config` パスから YAML を読み込みます。ファイルが存在しない場合は組み込みのデフォルト値が返されます。セクション：`server`、`database`、`jwt`、`ai`、`agent`。

### 4.3 `internal/router`

`router.go` は、すべてのルートが登録される唯一の場所です：

- 公開ルート（`/healthz`、`/auth/login`、`/auth/refresh` など）；
- 認証済みパネルルート（`/api/v1/*`）は `authMw + audit + accesslog` の背後に配置；
- 書き込みルートはさらに `permMw("permission.key")` で保護；
- agent ルート（`/agent/v1/*`）は `middleware.AgentAuth` の背後に配置；
- WebSocket ルート（`/terminal`、`/containers/:id/exec`）。

**規約：** すべての書き込みエンドポイント（POST/PUT/DELETE）は `permMw(...)` ガードを付ける必要があります。読み取りエンドポイントは、機密データを公開しない限り、ログイン済みのすべてのユーザーに開かれています。

### 4.4 `internal/middleware`

| ファイル | 目的 |
|----------|------|
| `auth.go` | JWT 認証。ユーザー id/role をコンテキストに注入 |
| `perm.go` | RBAC 権限チェック（`permMw`） |
| `audit.go` | 監査ログエントリを書き込む |
| `accesslog.go` | アクセスログエントリを書き込む |
| `cors.go` | CORS ヘッダー |
| `ratelimit.go` | ログインのレート制限 |
| `metrics_auth.go` | `/metrics` のベアラートークンガード |
| `agent.go` | `/agent/v1` の Agent Token 認証 |

### 4.5 `internal/api/v1`

モジュールごとに 1 つのハンドラーファイル。各ハンドラーは：

1. リクエストをバインド/検証；
2. 対応する service メソッドを呼び出し；
3. 統一された `response.OK` / `response.Error` を返す。

パネルハンドラーは `v1` パッケージにあります（例：`system.go`、`file.go`、`skill.go`）。agent ハンドラーは `agent.go` にあります。

### 4.6 `internal/service`

ビジネスロジックレイヤー。モジュールごとに 1 ファイル。主なモジュール：

| ファイル | モジュール |
|----------|-----------|
| `auth.go`、`user.go`、`role.go`、`totp.go` | 認証、ユーザー、RBAC、2FA |
| `system.go`、`metrics.go`、`prometheus.go` | ホスト情報、メトリクス、prometheus エクスポート |
| `website.go`、`database.go`、`cron.go`、`crontab.go` | リソース管理 |
| `file.go` | パストラバーサル保護付きファイル操作 |
| `docker.go` | Docker コンテナ/イメージ/ボリューム/ネットワーク |
| `ai.go` | LLM アダプター（チャット、ストリーミング） |
| `skill.go`、`mcp.go` | AI スキル + MCP サーバー |
| `alert.go`、`alertmonitor.go` | アラートルール + 評価 |
| `security.go`、`securitymonitor.go`、`baseline.go`、`fim.go`、`threat.go`、`firewall.go`、`loginsecurity.go` | ホストセキュリティスイート |
| `notification.go`、`audit.go`、`setting.go` | 通知、監査、設定 |
| `appstore.go`、`apptemplate.go`、`appcategory.go` | アプリストア / テンプレート |

### 4.7 `internal/repository`

GORM データアクセス。モデルごとに 1 ファイル。CRUD とクエリヘルパーを提供します。ビジネスロジックは含みません。

### 4.8 `internal/model`

SQLite テーブルにマッピングされた GORM 構造体と、RBAC 権限グループの唯一の権威ソースである `role.go`（`PermGroups()`、`AllPermKeys()`、`BuiltinRoles()`）。

### 4.9 `internal/pkg`

| パッケージ | 目的 |
|------------|------|
| `jwt` | access/refresh トークンの署名と検証 |
| `response` | 統一レスポンスエンベロープ `{code,message,data}` |
| `store` | SQLite のオープン、自動マイグレーション、シード（デフォルト admin） |

### 4.10 `web`

- `index.html` — 単一ファイル Vue3 SPA：7 言語 i18n、テーマシステム、ログイン、ダッシュボード、監視、セキュリティ、ファイル、ターミナル、AI アシスタント、設定。
- `embed.go` — `go:embed` によるフロントエンドのバイナリ埋め込み。
- `static/` — オフラインのフロントエンド依存関係（Vue、ECharts）。

---

## 5. API 設計

### 5.1 レスポンスエンベロープ

すべてのエンドポイントが返します：

```json
{ "code": 0, "message": "ok", "data": { } }
```

- `code == 0` → 成功、`data` がペイロードを保持；
- `code != 0` → ビジネスエラー、`message` がその内容を説明。

### 5.2 認証

- パネル API（`/api/v1`）：JWT アクセストークン（`Authorization: Bearer <token>`）、有効期限が短く、`/auth/refresh` で更新。
- Agent API（`/agent/v1`）：静的な Agent Token（設定 `agent.token`）。

### 5.3 エンドポイントファミリー

| ファミリー | 対象 | 認証 |
|------------|------|------|
| `/api/v1/*` | ブラウザー UI | ユーザー JWT + RBAC |
| `/agent/v1/*` | 外部システム / プロキシ | Agent Token |

---

## 6. データモデル

モデルは `internal/model` 内の GORM 構造体です。テーブルは起動時に `internal/pkg/store` によって自動マイグレーションされます。代表的なモデル：

- `User`（id、username、パスワードハッシュ、role、MFA シークレット、...）
- `Role`（name、label、perms、builtin）
- `Website`、`Database`、`CronJob`
- `AlertRule`、`AlertEvent`、`Notification`
- `McpServer`、`Skill`（スキルはディスク上のディレクトリとして保存、DB ではない）
- `AuditLog`、`AccessLog`、`Setting`
- セキュリティ：`BaselineResult`、`FimBaseline`、`FimChange`、`ThreatFinding`

---

## 7. RBAC と権限

権限グループは `internal/model/role.go`（`PermGroups()`）で定義され、唯一の情報源です。3 つの組み込みロール：

- **admin** — 権限 `"*"`（すべて）；
- **operator** — `user.*` と `settings.edit` を除くすべての権限；
- **readonly** — `*.view` 権限のみ。

フロントエンドのメニュー/ボタンは `hasPerm('key')` で同じキーにより制御され、バックエンドは `permMw("key")` で強制します。新しい書き込み機能を追加するときは、**必ず** 3 か所すべてを変更する必要があります：

1. `PermGroups()` に権限キーを追加；
2. `permMw(...)` でルートを保護；
3. `hasPerm(...)` でボタンを保護。

---

## 8. ビルドとデプロイ

```bash
make build          # 現在のプラットフォーム向けにビルド
make build-all      # すべてのプラットフォーム向けにクロスコンパイル
make version        # バージョン情報を表示
```

バイナリは静的です（CGO なし）。systemd、リバースプロキシ、アップグレード手順については [`build-and-deploy.md`](build-and-deploy.md) を参照してください。

---

## 9. 開発ガイド

### 9.1 新しいモジュールの追加

レイヤー化パターンに従い、作成（または拡張）します：

1. `internal/model/xxx.go` — GORM 構造体；
2. `internal/repository/xxx.go` — データアクセス；
3. `internal/service/xxx.go` — ビジネスロジック；
4. `internal/api/v1/xxx.go` — ハンドラー；
5. `internal/router/router.go` にルートを登録。

### 9.2 新しい書き込みエンドポイントの追加

1. `internal/model/role.go` に権限キーを追加；
2. `permMw("...")` でルートを登録；
3. フロントエンドのボタンに `hasPerm("...")` ガードを追加；
4. `web/index.html` に i18n キーを追加（全 7 言語）。

### 9.3 i18n 規約

フロントエンドの i18n 辞書（`I18N`...`I18N8`）は 7 言語を保持します：`zh-CN`、`zh-TW`、`en`、`ja`、`ko`、`th`、`de`。新しいキーはすべて**7 言語すべて**に追加する必要があります — `t(key)` ヘルパーは欠落時に `zh-CN` へフォールバックしますが、翻訳が欠けていると非中国語ユーザーに中国語が表示されます。

### 9.4 コードスタイル

- エクスポートされる Go の関数/型には、その名前で始まるドキュメントコメントを付けます。
- コメントは英語で書きます。
- 各ディレクトリには、そのファイルを説明する `README.md` があります。
