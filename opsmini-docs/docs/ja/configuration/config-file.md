# メイン設定ファイル

OpsMini は YAML 設定ファイルを使用します。公式のワンクリックインストール後はデフォルトで `/data/opsmini/config.yaml` に配置されます。ソースコードから実行する場合はデフォルトで `configs/config.yaml` を使用し、いずれも `-config` パラメータで指定できます。設定ファイルが存在しない場合、プログラムは内蔵のデフォルト値で起動します。

公式で同梱されるサンプルは**最小限の実行可能設定**に簡素化されており、その他のランタイム設定（AI、Agent トークン、監視認証など）はパネルの「設定」ページに移行しています。

```yaml
# OpsMini Agent 設定ファイル
server:
  host: "0.0.0.0"        # リッスンアドレス
  port: 8888              # リッスンポート
  secret_entry: ""        # セキュリティ入口のパスプレフィックス。例：/opsmini_panel（空なら無効）

database:
  path: "opsmini.db"      # SQLite データファイルのパス

jwt:
  access_ttl_seconds: 900     # access token の有効期間（15 分）
  refresh_ttl_seconds: 604800 # refresh token の有効期間（7 日）

log:
  level: error               # ログレベル：info / warn / error
  path: ""                   # ログファイルのパス。空なら stdout（systemd journal）のみに出力
```

## 設定項目の説明

### server

| パラメータ | 説明 | デフォルト |
|------|------|------|
| `host` | リッスンアドレス。`0.0.0.0` はすべての NIC を表す | `0.0.0.0` |
| `port` | リッスンポート | `8888` |
| `secret_entry` | セキュリティ入口のパスプレフィックス。例：`/opsmini_panel`。空なら無効 | 空 |

`secret_entry` を設定すると、パネルとすべての API はプレフィックスパスの配下に配置されます（例：`http://<host>:8888/opsmini_panel`）。リバースプロキシと組み合わせて実際の入口を隠し、ポートスキャンやブルートフォースプローブを防げます。

### database

| パラメータ | 説明 | デフォルト |
|------|------|------|
| `path` | SQLite データファイルのパス | `opsmini.db` |

### jwt

| パラメータ | 説明 | デフォルト |
|------|------|------|
| `access_ttl_seconds` | access token の有効期間（秒） | `900` |
| `refresh_ttl_seconds` | refresh token の有効期間（秒） | `604800` |

> JWT 署名シークレットは**もはや**ここでは設定しません。初回起動時に 32 バイトのランダムシークレットを自動生成してデータベースに永続化し、設定ファイルにも UI にも残さないため、手動での管理は不要です。

### log

| パラメータ | 説明 | デフォルト |
|------|------|------|
| `level` | ログレベル：`info` / `warn` / `error` | `error` |
| `path` | ログファイルのパス。空なら stdout（systemd journal）のみに出力 | 空 |

ログレベルは出力の詳細度を制御します：`error` はエラーのみ出力（本番推奨。SQL クエリログで画面が埋まるのを回避）。`warn` はさらにスロークエリと警告を出力。`info` は全ログを出力（SQL クエリを含む。問題切り分けに適す）。ワンクリックインストールではデフォルトで `/data/opsmini/opsmini.log` に書き込みます。

## 任意の設定セクション

以下のフィールドは設定構造では引き続きサポートされていますが、公式サンプルでは省略されています（内蔵のデフォルト値を使用）。必要に応じて明示的に宣言できます。

### agent（コマンドホワイトリスト）

```yaml
agent:
  allowed_commands:        # Agent API コマンドのホワイトリストプレフィックス
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

| パラメータ | 説明 | デフォルト |
|------|------|------|
| `allowed_commands` | `/agent/v1/commands` で実行を許可するコマンドプレフィックスのホワイトリスト | 上記参照 |

> `agent.token` は廃止されました。Agent API の認証トークンはパネルの「設定 → API Token」ページで生成・管理します。空にすると Agent API は無効化されます。

### metrics（Prometheus 指標）

```yaml
metrics:
  enabled: true     # /metrics エンドポイントを有効にするか
  user: ""          # Basic 認証ユーザー名。空なら認証なし
  password: ""      # Basic 認証パスワード
```

| パラメータ | 説明 | デフォルト |
|------|------|------|
| `enabled` | `/metrics` エンドポイントを有効にするか | `true` |
| `user` | HTTP Basic 認証ユーザー名。空なら認証なし | 空 |
| `password` | HTTP Basic 認証パスワード | 空 |

> 認証情報の優先順位：パネル「設定 → 監視エクスポート」のユーザー名/パスワードがここでの `user`/`password` **より優先**されます。ユーザー名を空にするとアクセスが開放されます。

## パネル設定へ移行済みの設定

以下の設定項目は `config.yaml` から移行され、パネルの「設定」ページで一元管理されます（SQLite に保存され、ランタイムで即時反映）：

| 元の設定 | 現在の管理場所 | 説明 |
|--------|-----------|------|
| `jwt.secret` | 自動生成（管理不要） | 初回起動時にランダムシークレットを生成して DB に保存 |
| `ai.*` | 設定 → AI 大規模モデル接続 | モデル / API Key / Base URL / 有効スイッチ |
| `agent.token` | 設定 → API Token | Agent API のアクセストークン。空なら無効 |
| `metrics.user/password` | 設定 → 監視エクスポート | ランタイムで設定ファイルの値を上書き可能 |

詳しくは [パネル設定](panel-settings.md) を参照してください。

## 環境変数

| 変数 | 説明 |
|------|------|
| `OPSMINI_AI_KEY` | AI API Key。**優先度が最も高い**（パネル設定・設定ファイルより優先。シークレットのディスク保存を回避） |

## コマンドラインパラメータ

| パラメータ | 説明 |
|------|------|
| `-config <path>` | 設定ファイルのパスを指定（ワンクリックインストールはデフォルト `/data/opsmini/config.yaml`、ソースコード実行はデフォルト `configs/config.yaml`） |
| `-version` | バージョン情報を表示して終了 |
| `-reset-mfa <username>` | 指定ユーザーの MFA バインディングをリセット（検証コード紛失時に使用）。完了後に終了 |
| `-reset-pass <username>` | 指定ユーザーのパスワードをランダムな強パスワードにリセットして表示。完了後に終了 |

> `-reset-mfa` / `-reset-pass` はデータベースを直接操作し、サービスは起動しません。アカウント紛失時の復旧に使用します。
