# パネル API（/api/v1）

パネル API はブラウザ UI 向けで、ユーザーセッション JWT + RBAC 認証を使用します。

## 認証

- ログイン API `POST /auth/login` が `access` + `refresh` トークンを返します
- 以降のリクエストには `Authorization: Bearer <access_token>` を付与します
- access の期限切れ後は refresh で新しいトークンに交換します
- ログインにはレート制限（ブルートフォース対策）があります。2FA を有効にすると、ログイン時にまず `POST /auth/mfa/verify` で動的コードを検証する必要があります

## 統一レスポンス

```json
{ "code": 0, "message": "ok", "data": { } }
```

`code` が 0 以外の場合はビジネスエラーです。

## エンドポイント一覧

### 認証とユーザー

| メソッド | パス | 説明 |
|------|------|------|
| POST | `/auth/login` | ログイン（レート制限） |
| POST | `/auth/refresh` | トークン更新 |
| POST | `/auth/logout` | ログアウト |
| POST | `/auth/mfa/verify` | ログイン時の MFA 動的コード検証 |
| GET | `/auth/mfa/status` | 現在のユーザーの MFA バインディング状態を照会 |
| POST | `/auth/mfa/setup` | MFA バインディング用 QR コード/シークレットを生成 |
| POST | `/auth/mfa/enable` | 検証して MFA を有効化 |
| POST | `/auth/mfa/disable` | MFA を解除 |
| GET | `/profile` | 個人プロフィール（ニックネーム/アバター/メール） |
| PUT | `/profile` | 個人プロフィールを変更 |
| POST | `/profile/password` | パスワード変更 |
| GET/POST/PUT/DELETE | `/users` | ユーザー CRUD |
| GET/POST/PUT/DELETE | `/roles` | ロール CRUD |
| GET | `/roles/groups` | 権限グループ |
| GET | `/permissions` | 現在のユーザーの権限 |

### パネル設定

| メソッド | パス | 説明 |
|------|------|------|
| GET | `/settings` | 非センシティブな設定をすべて取得（`settings.view`） |
| PUT | `/settings` | 設定を一括更新（`settings.edit`） |

> センシティブ項目（`jwt_secret`、`metrics_pass`）は返されません。書き込み保護項目（`jwt_secret`）は上書きできません。詳しくは [パネル設定](../configuration/panel-settings.md) を参照してください。

### 監視とアラート

| メソッド | パス | 説明 |
|------|------|------|
| GET | `/dashboard/overview` | ダッシュボード概要 |
| GET | `/dashboard/metrics` | ダッシュボード指標 |
| GET | `/system/monitor` | 監視サマリー |
| CRUD | `/alert-rules` | アラートルール |
| GET | `/alert-events` | アラートイベント |

### 通知

| メソッド | パス | 説明 |
|------|------|------|
| GET | `/notifications` | 通知一覧 |
| GET | `/notifications/unread-count` | 未読数 |
| PUT | `/notifications/read-all` | すべて既読 |
| PUT | `/notifications/:id/read` | 既読にする |
| DELETE | `/notifications/:id` / `/notifications` | 単件削除 / 全消去 |

### リソース管理

| メソッド | パス | 説明 |
|------|------|------|
| CRUD | `/websites` | ウェブサイト |
| CRUD | `/databases` | データベース |
| GET/POST | `/apps` `/app-categories` | アプリストアとカテゴリ |
| GET/POST | `/containers` `/images` `/volumes` `/networks` | コンテナ 4 種 |
| GET/POST | `/files` | ファイル |
| CRUD | `/cron-jobs` | スケジュールタスク |
| GET | `/logs` `/logs/tail` | ログ |

### システムとセキュリティ

| メソッド | パス | 説明 |
|------|------|------|
| GET | `/system/info` `/processes` `/ports` `/disks` `/network` `/users` `/groups` `/firewall` | システムリソース |
| GET/POST | `/security/...` | ホストセキュリティ（ベースライン / FIM / 脅威 / ファイアウォール / ログインセキュリティ） |
| GET | `/audit-logs` `/access-logs` | 監査 / アクセスログ |

### AI と連携

| メソッド | パス | 説明 |
|------|------|------|
| POST | `/ai/chat` | AI チャット（関数呼び出し） |
| POST | `/ai/chat/stream` | AI チャット（ストリーミング SSE） |
| GET/POST/PUT/DELETE | `/mcp` | MCP 設定 |
| GET/POST/DELETE | `/skills` など | AI スキル（SkillHub 検索/インストール/アップロード） |

### ターミナル

| メソッド | パス | 説明 |
|------|------|------|
| GET | `/terminal` | WebSocket ターミナル（query token 認証） |
| GET | `/containers/:id/exec` | コンテナ WebSocket ターミナル |

## 説明

- すべての書き込み操作は RBAC 権限ポイントで制御されます（例：`website.create`、`container.edit`、`settings.edit`）
- 重要な操作は監査ログに記録され、ログイン状態のリクエストはアクセスログに記録されます
