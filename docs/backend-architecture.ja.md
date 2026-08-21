<div align="center">

# OpsMini Backend Architecture

**バックエンドアーキテクチャ設計**

[English](backend-architecture.md) · [简体中文](backend-architecture.zh-CN.md) · [繁體中文](backend-architecture.zh-TW.md) · [日本語](backend-architecture.ja.md) · [한국어](backend-architecture.ko.md) · [ไทย](backend-architecture.th.md) · [Deutsch](backend-architecture.de.md)

</div>

---

> バージョン：v1.0  
> 参照：`web/index.html` UI プロトタイプ（ログイン／ユーザー／ダッシュボード／監視／アプリ／コンテナ／システム／ファイル／ターミナル／cron ジョブ／ログ／AI アシスタント／パネル設定／7 言語 i18n）

---

## 1. 概要

OpsMini は**軽量な単一マシン運用パネル**（宝塔 / 1Panel に対抗）で、2 つの差別化ポイントがあります：

1. **内蔵 AI 大規模モデル**によるシステム管理（自然言語による診断 / 実行 / ログ分析）；
2. **標準 REST API を外部に公開**し、外部システム（監視プラットフォーム、自動化スクリプト、サードパーティのオーケストレーションツール）から統合して呼び出せます。

> v1.0 のスコープ説明：本バージョンには **Proxy 中央ノードは含まれません**。各ホスト上の OpsMini パネルは独立して動作し、
> 標準 REST API を通じて外部に機能を提供します。複数マシンの統合管理（Proxy）は後続バージョンで評価します。

### 1.1 設計目標

| 目標 | 説明 |
|------|------|
| 軽量 | 単一バイナリでデプロイ、メモリ使用量が少なく、1C1G の小型ホストに最適 |
| 単一マシン優先 | 中核シナリオは単一サーバーで、分散の複雑さを持ち込まない |
| 統合可能 | 標準 REST API で機能を公開し、外部システムから統合呼び出し可能 |
| 安全 | RBAC、2FA、セキュアエントリ、最小権限、鍵の暗号化保存 |
| 保守性 | モジュール化されたレイヤー、Controller → Service → Repository が明確 |

### 1.2 技術スタック選定

| レイヤー | 選定 | 代替 | 理由 |
|----|------|------|------|
| 言語 | Go 1.22+ | — | 単一バイナリ、クロスコンパイル、並行性に優れ、エコシステムが成熟（1Panel と同じスタック） |
| Web フレームワーク | **Gin** | Echo / chi | 最大のエコシステム、豊富なミドルウェア、1Panel と同じ |
| データベース | **SQLite** | — | 組み込み、ゼロ運用、単一マシンのシナリオに適合 |
| SQLite ドライバ | **modernc.org/sqlite** | mattn/go-sqlite3 | 純 Go で CGO 不要、クロスコンパイルが容易 |
| ORM | **GORM** | sqlx | 開発効率が高い。複雑なクエリはネイティブ SQL にフォールバック可能 |
| リアルタイム通信 | **gorilla/websocket** | — | ターミナル、ログ tail、指標プッシュ |
| タスクスケジューリング | **robfig/cron** | — | パネルの cron ジョブ |
| システム監視 | **gopsutil** | /proc 読み取り | クロスプラットフォームの CPU/メモリ/ディスク/プロセス |
| Docker | 公式 SDK | — | コンテナ/イメージ/ボリューム/ネットワーク |
| ログ | zerolog | zap | 軽量、構造化、低アロケーション |
| 認証 | JWT + refresh | session | ステートレス API + オプションのセッション |
| 2FA | TOTP（RFC 6238） | — | pquerna/otp を再利用 |
| AI | 抽象 LLM インターフェース | — | OpenAI/DeepSeek/Qwen/Ollama を統一的にアダプト |

### 1.3 全体アーキテクチャ

```
┌────────────────────────────────────────────────────────────┐
│                OpsMini（单机面板，单二进制）                    │
│                                                            │
│   Vue3 SPA（构建产物 embed 进二进制）                          │
│        │  HTTP / WebSocket                                  │
│   ┌────▼────────────────────────────────────────────────┐   │
│   │  Gin Router → 中间件（认证/权限/审计/限流/安全入口）     │   │
│   │  Controller（参数校验）→ Service（业务）→ Repo（GORM）  │   │
│   └────┬────────────────────────────────────────────────┘   │
│        │                                                    │
│   ┌────▼────────────────────────────────────────────────┐   │
│   │  SQLite（业务数据）   │   系统资源适配层                │   │
│   │  users/cron/websites │   Docker SDK / crontab /       │   │
│   │  ...                 │   gopsutil / 文件系统 / SSH    │   │
│   └──────────────────────┴───────────────────────────────┘   │
│                                                              │
│   对外暴露两套标准 REST API：                                   │
│   · /api/v1   面板 API（浏览器，用户 JWT + RBAC）              │
│   · /agent/v1 Agent API（机器，Agent Token，命令白名单）       │
└──────────────────────────────────────────────────────────────┘
```

---

## 2. 中核アーキテクチャの決定

### 2.1 モノリス単一バイナリ、マイクロサービス化しない

**決定**：単一プロセスのモノリス。フロントエンドの Vue3 ビルド成果物は `go:embed` でバイナリに埋め込み、最終的に単一の `opsmini` 実行ファイルをデリバリします。

**理由**：
- 運用パネルの中核要件は「1 台にインストールして、その 1 台を管理する」ことで、モノリスが最も適合します；
- 外部統合は標準 REST API で行うため、モノリスと矛盾しません；
- マイクロサービスはデプロイ、サービスディスカバリ、分散トランザクションなどの無駄な複雑さをもたらします。

### 2.2 フロントエンド／バックエンド分離 + 埋め込みパッケージ

- 開発時：Vue3 dev server が Gin へプロキシ（CORS / リバースプロキシ）；
- 本番時：`web/dist` の成果物を `go:embed` でバイナリに埋め込み、単一ファイルでデプロイ。

### 2.3 MySQL/Postgres ではなく SQLite

- 業務データ量は小さく（設定、ジョブ、ユーザー、ログ）、SQLite で十分です；
- ゼロ運用、単一ファイル、バックアップが容易（.db を直接コピー）；
- 将来パネル自身のデータに高並行性が必要になった場合、Repository インターフェースを抽象化して切り替え可能。

### 2.4 標準 REST API の外部公開

- v1.0 では **Proxy 中央ノードを開発せず**、パネルの機能のみを標準 REST API として公開します；
- 2 つの API が併存：`/api/v1`（ブラウザ UI 向け）と `/agent/v1`（マシン／サードパーティ統合向け）；
- Agent API は独立認証（Agent Token）+ コマンド許可リストで、ユーザーセッション JWT と分離；
- 将来複数マシンの統合管理が必要な場合、Agent API の上に Proxy のプル・スケジューリング層を追加可能（v1.0 の範囲外）。

---

## 3. モジュール分割（UI プロトタイプに対応）

| バックエンドモジュール | 責務 | 対応するプロトタイプページ |
|----------|------|-------------|
| `auth` | ログイン/ログアウト、セッション、2FA、RBAC | ログインページ、ユーザー管理 |
| `setting` | パネル設定、テーマ、メニュー表示、言語 | パネル設定（基本/外観/メニュー） |
| `dashboard` | 指標集約、リアルタイムプッシュ | ダッシュボード |
| `monitor` | 時系列収集、アラートルール、アラート発火 | 監視 |
| `website` | Nginx サイト、ドメイン、SSL 証明書 | アプリ管理-Web サイト |
| `database` | MySQL/PostgreSQL インスタンスと DB | アプリ管理-データベース |
| `store` | ソフトウェアのインストール/アンインストール/アップグレード | アプリ管理-ソフトウェアストア |
| `container` | Docker コンテナ/イメージ/ボリューム/ネットワーク | コンテナ管理 |
| `system` | プロセス/ネットワーク/ポート/ディスク | システム管理 |
| `file` | ファイル閲覧/アップロード/編集/権限 | ファイル |
| `terminal` | Web SSH | ターミナル |
| `cron` | cron ジョブ（パネル+システム+ユーザー crontab） | cron ジョブ |
| `log` | ログ収集/集約/tail | ログ |
| `ai` | LLM 接続、コンテキスト、NL 実行 | AI アシスタント、パネル設定-AI |
| `agent` | 外部 REST API（`/agent/v1`）、Agent Token 認証、コマンド許可リスト | パネル設定-Proxy 接続 |

---

## 4. レイヤーとディレクトリ構造

### 4.1 レイヤー

各モジュール内部は厳密に 3 層：

```
Controller（HTTP handler）：参数解析校验、调用 Service、组装响应
    → Service（业务逻辑）：编排 Repository 与系统资源、事务边界
    → Repository（GORM）：纯数据访问，不写业务
```

### 4.2 ディレクトリ構造

```
opsmini/
├── cmd/
│   └── agent/main.go          # 单机面板主程序
├── internal/
│   ├── router/                # 路由注册、API 版本
│   ├── middleware/            # 认证、权限、审计、限流、安全入口
│   ├── api/v1/                # Controller（按模块分包，含 /api/v1 与 /agent/v1）
│   ├── service/               # Service（按模块分包）
│   ├── repository/            # Repository（按模块分包）
│   ├── model/                 # GORM 数据模型
│   ├── agent/                 # Agent REST API（对外暴露、token 认证、命令白名单）
│   ├── ai/                    # LLM 适配层（provider 接口 + 各实现）
│   └── pkg/                   # 通用工具
│       ├── config/            # 配置加载（文件+环境变量）
│       ├── logger/            # zerolog 封装
│       ├── jwt/               # token 签发/校验
│       ├── otp/               # 2FA TOTP
│       ├── sysinfo/           # gopsutil 封装（采集指标）
│       ├── crontab/           # 系统/用户 crontab 读写
│       ├── docker/            # Docker SDK 封装
│       └── store/             # SQLite 连接 + 迁移
├── web/                       # Vue3 前端源码（构建后 embed）
├── docs/
├── configs/                   # 默认配置示例
└── go.mod
```

---

## 5. データモデル（SQLite スキーマ）

> GORM マイグレーション。機密フィールド（鍵/Token）は AES-GCM で暗号化して保存。

```sql
-- 用户与认证
CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,          -- bcrypt
  role          TEXT NOT NULL DEFAULT 'operator',  -- admin/operator/readonly
  auth_method   TEXT NOT NULL DEFAULT 'password',  -- password/2fa
  totp_secret   TEXT,                    -- 加密存储
  status        INTEGER NOT NULL DEFAULT 1,        -- 1启用 0停用
  last_login    TEXT,
  created_at    TEXT,
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL,
  refresh    TEXT NOT NULL UNIQUE,
  expire_at  TEXT NOT NULL,
  created_at TEXT
);

-- 面板配置（KV，含主题/语言/菜单显隐/代理）
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT
);

-- 网站
CREATE TABLE websites (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  domain     TEXT NOT NULL UNIQUE,
  path       TEXT NOT NULL,
  env        TEXT,                       -- nginx/static
  runtime    TEXT,                       -- php8.2/php8.1/node20/static
  ssl        INTEGER DEFAULT 0,
  ssl_days   INTEGER,
  status     INTEGER DEFAULT 1,
  created_at TEXT
);

-- 数据库实例
CREATE TABLE databases (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  type      TEXT NOT NULL,               -- mysql/postgresql
  name      TEXT NOT NULL,
  charset   TEXT,
  created_at TEXT
);

-- 计划任务（含系统/用户 crontab 的只读映射）
CREATE TABLE cron_jobs (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  name      TEXT NOT NULL,
  kind      TEXT NOT NULL,               -- panel/system/user
  user      TEXT,
  src_path  TEXT,                        -- /etc/crontab 或 sqlite 等
  schedule  TEXT NOT NULL,               -- cron 表达式
  command   TEXT NOT NULL,
  enabled   INTEGER DEFAULT 1,
  last_run  TEXT,
  created_at TEXT
);

-- 告警规则
CREATE TABLE alert_rules (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  name      TEXT NOT NULL,
  metric    TEXT NOT NULL,               -- cpu/mem/disk/service
  condition TEXT NOT NULL,               -- >90% 等
  duration  TEXT,
  notify    TEXT,
  enabled   INTEGER DEFAULT 1,
  created_at TEXT
);

-- Agent API 配置
CREATE TABLE agent_config (
  id        INTEGER PRIMARY KEY CHECK (id = 1),  -- 单行
  agent_id  TEXT,
  token     TEXT,                        -- 加密存储（对外 REST API 认证）
  allowed_commands TEXT,                 -- 命令白名单（逗号分隔）
  enabled   INTEGER DEFAULT 0
);

-- 操作/审计日志
CREATE TABLE audit_logs (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER,
  action     TEXT,
  target     TEXT,
  detail     TEXT,
  created_at TEXT
);
```

---

## 6. API 設計（RESTful、`/api/v1`）

統一レスポンス：`{ "code": 0, "message": "ok", "data": ... }`、`code` が 0 以外は業務エラー。

| メソッド | パス | 説明 |
|------|------|------|
| POST | `/auth/login` | ログイン（access + refresh を返す） |
| POST | `/auth/refresh` | トークンをリフレッシュ |
| POST | `/auth/logout` | ログアウト |
| GET | `/auth/2fa/qrcode` | 2FA QR コードを生成 |
| POST | `/auth/2fa/verify` | 2FA を検証 |
| GET | `/users` / POST / PUT / DELETE | ユーザー CRUD |
| GET | `/dashboard/metrics` | ダッシュボード指標 |
| GET | `/monitor/series?range=1h` | 時系列データ |
| CRUD | `/alert-rules` | アラートルール |
| CRUD | `/websites` | Web サイト |
| POST | `/websites/:id/ssl` | SSL 発行/更新 |
| CRUD | `/databases` | データベース |
| GET | `/store/apps` / POST `/store/apps/:id/install` | ソフトウェアストア |
| GET | `/containers` / `/images` / `/volumes` / `/networks` | コンテナ 4 種 |
| POST | `/containers` など | コンテナ作成/イメージ pull/ボリューム作成/ネットワーク作成 |
| GET | `/system/processes` `/networks` `/ports` `/disks` | システムリソース |
| GET/POST | `/files` / `/files/list` / `/files/upload` / `/files/edit` | ファイル |
| WS  | `/terminal/ws?cols=&rows=` | Web SSH |
| CRUD | `/cron-jobs` | cron ジョブ（パネル類） |
| GET | `/cron-jobs/system` `/cron-jobs/user` | システム/ユーザー crontab（読み取り専用） |
| GET | `/logs` | ログ一覧 |
| POST | `/ai/chat` | AI 対話（ストリーミング SSE） |
| POST | `/ai/execute` | NL から操作へ（権限確認付き） |
| GET/PUT | `/agent/config` | Agent API 設定（token、コマンド許可リスト） |
| GET/PUT | `/settings` | パネル設定 |
| GET | `/i18n/{lang}` | 言語パック（フロントエンドでもインライン可能） |
| GET | `/metrics` | **Prometheus 指標**（node_exporter 互換、`/api/v1` プレフィックスなし） |

### 6.1 Prometheus 監視統合

OpsMini は **node_exporter 互換の `/metrics` エンドポイント**を内蔵しており、node_exporter を別途インストールする必要はなく、Prometheus が直接スクレイプできます：

```
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
```

node_exporter の主要指標にアライン済み（コミュニティの Node Dashboard をそのまま適用可能）：

| 指標ファミリー | 説明 |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | コアごと・モードごとの累計秒数 |
| `node_memory_MemTotal_bytes` など | メモリ Total/Free/Available/Buffers/Cached |
| `node_filesystem_size_bytes{mountpoint}` | 各マウントポイントの容量/空き/使用率 |
| `node_network_receive_bytes_total{device}` | 各 NIC の送受信トラフィック |
| `node_load1` / `node_load5` / `node_load15` | ロードアベレージ |
| `node_uname_info` / `node_boot_time_seconds` | ホスト情報と起動時刻 |

> 実装方法：`gopsutil`（SystemService が収集済みのデータ）を再利用し、Prometheus テキスト形式で出力、
> 単一バイナリデリバリを維持し、node_exporter プロセスを埋め込まない。

### 6.2 Agent API（外部向け標準 REST インターフェース）

パネルは UI が使用する `/api/v1` に加えて、**独立したマシン間 REST API**（`/agent/v1`）を公開し、外部システム（監視プラットフォーム、自動化スクリプト、サードパーティのオーケストレーションツール）から統合して呼び出せます。パネル API との違い：

- **認証**：ユーザーセッション JWT ではなく Agent Token を使用；
- **範囲**：リソース管理とコマンド実行にフォーカスし、UI 専用機能（i18n/テーマ/メニュー）とターミナル WS は含まない；
- **スタイル**：自動化向け——冪等、再試行可能、統一 JSON レスポンス。

| メソッド | パス | 説明 |
|------|------|------|
| GET | `/agent/v1/health` | ヘルスチェック（死活監視） |
| GET | `/agent/v1/status` | ステータスサマリ：cpu/mem/ディスク/オンラインサービス |
| GET | `/agent/v1/system/info` | ホスト情報（hostname/os/カーネル） |
| GET | `/agent/v1/system/processes` | プロセス一覧 |
| GET | `/agent/v1/system/ports` | ポートリスニング |
| GET | `/agent/v1/system/disks` | ディスク/マウントポイント |
| GET | `/agent/v1/websites` / POST | Web サイトの照会/作成 |
| GET | `/agent/v1/databases` | データベース一覧 |
| GET | `/agent/v1/containers` | コンテナ一覧 |
| GET | `/agent/v1/cron-jobs` | cron ジョブ |
| POST | `/agent/v1/commands` | コマンド実行（許可リスト、実行結果を返す） |

---

## 7. 外部 REST API 設計（差別化の重点）

### 7.1 位置づけ

v1.0 の OpsMini は**単一マシンパネル**で、標準 REST API を通じて外部に機能を公開し、外部システムの統合に供します：

- **`/api/v1`（パネル API）**：ブラウザ UI 向け、ユーザーセッション JWT + RBAC；
- **`/agent/v1`（Agent API）**：マシン／サードパーティ統合向け、Agent Token 認証 + コマンド許可リスト。

> salt minion/master の「アウトバウンド長接続 push」とは異なり、OpsMini は**REST を公開して外部からプル呼び出し**させます。
> Prometheus が exporter をプルする方式やクラウドベンダーの OpenAPI の考え方に近いものです。Proxy センターを導入して複数マシンを統合管理するかは、後続バージョンで評価します（v1.0 では行いません）。

### 7.2 呼び出しモデル

```
         HTTPS REST 调用（Agent Token）
   ┌──────────┐  ─────────────────────▶  ┌─────────┐
   │ 外部系统  │                          │ OpsMini │
   │ (监控/脚本 │  ◀─────────────────────  │ (单机面板)│
   │ /编排工具) │       统一 JSON 响应      └─────────┘
   └──────────┘
```

- **長接続なし**：すべて標準 REST 経由で、WebSocket / gRPC の長接続を維持しません；
- **ストリーミングシナリオ**（ログ tail、リアルタイム指標）：REST のページングポーリングで十分で、SSE は不要；
- **冪等性**：参照系 GET は自然に冪等。書き込み操作（コマンド実行、リソース作成）は明確な結果を返します。

### 7.3 主要フロー

1. パネル起動 → `agent_config`（token + コマンド許可リスト）を読み込み；
2. 外部システムが `Authorization: Bearer <token>` を付与して `/agent/v1/*` を呼び出し；
3. ミドルウェアが token を検証（定数時間比較）→ 許可リストに一致すれば通過、そうでなければ 401；
4. 高リスク操作（コマンド実行）はコマンド許可リストの二次検証を経て、すべて監査に記録；
5. 統一 JSON を返す：`{ code, message, data }`。

### 7.4 セキュリティ

- **認証**：パネルは独立したランダムな Agent Token を設定（暗号化保存）、リクエストヘッダー `Authorization: Bearer <token>`；
- **転送**：HTTPS。高セキュリティシナリオでは IP 許可リストを追加可能；
- **コマンド許可リスト**：`/commands` は明示的に許可されたコマンドのみ許可し、すべて監査に記録；
- **最小露出**：`/agent/v1` と `/api/v1` を分離し、Agent API は UI 機能とターミナルを公開しない。

### 7.5 2 つの API の位置づけ

| 次元 | パネル API `/api/v1` | Agent API `/agent/v1` |
|------|-------------------|----------------------|
| 利用者 | ブラウザ（人） | 外部システム（マシン） |
| 認証 | ユーザーセッション JWT + RBAC | Agent Token |
| 範囲 | 全 UI 機能 + ターミナル WS | リソース管理 + コマンド実行（UI/ターミナルなし） |
| 設計 | 対話指向 | 自動化指向（冪等、再試行可能） |

---

## 8. セキュリティ設計

| 次元 | 方式 |
|------|------|
| パスワード | bcrypt ハッシュ |
| セッション | 短期 access JWT（15min）+ refresh token（失効可能） |
| 2FA | TOTP（RFC 6238）、ログイン時に任意の二要素検証 |
| 認可 | RBAC 3 ロール：admin（すべて）/ operator（日常運用）/ readonly（読み取り専用） |
| セキュアエントリ | パネルアクセスに secret パス（例 `/opsmini_panel`）が必要で、ポートスキャンを防止 |
| 鍵の保存 | パネル鍵（API Key、Agent Token）を AES-GCM で暗号化して保存 |
| ターミナル | Web SSH は operator 以上にのみ開放し、セッションを記録・監査 |
| ブルートフォース対策 | ログイン失敗のレート制限 + ロックアウト |
| 監査 | 重要操作は `audit_logs` に記録 |
| CSRF/XSS | API は Cookie なしの Bearer Token を使用。フロントエンドはエスケープ |

---

## 9. 主要フロー

### 9.1 ログイン

```
输入账密 → bcrypt 校验 → 若开启 2FA 则要求 TOTP
  → 签发 access + refresh → 前端存 refresh（httpOnly/localStorage）
  → 后续请求带 Bearer access → 过期用 refresh 换新
```

### 9.2 Web SSH ターミナル

```
前端 ws://host/api/v1/terminal/ws?token=...
  → 服务端校验 token + 角色
  → 启动 pty（github.com/creack/pty）→ 双向数据转发
  → 关闭时回收 pty、记录会话时长
```

### 9.3 cron ジョブ実行

- **パネルジョブ**：robfig/cron が常駐スケジューリングし、`cron_jobs` に書き込み；
- **システム/ユーザージョブ**：`/etc/crontab`、`/etc/cron.d/`、`/var/spool/cron/<user>` を直接読み書き（読み取り専用表示 + 制御された編集）。

### 9.4 AI アシスタント

```
用户输入 → Service 拼上下文（当前页面/模块 + 系统状态）
  → 调 LLM（provider 适配：OpenAI/DeepSeek/Qwen/Ollama）
  → 若模型判定为「执行意图」→ 生成结构化 action + 参数
  → 命中权限白名单 → 执行 → 回传结果
  → 未授权/高危 → 要求用户二次确认
```

### 9.5 指標収集

- コレクター：gopsutil が 5 秒ごとにサンプリング → メモリ上のリングバッファ；
- 履歴：ダウンサンプリング後に SQLite へ保存（1 分粒度で 7 日間保持）；
- プッシュ：WebSocket でダッシュボード/監視ページの購読者にブロードキャスト。

---

## 10. 監視と可観測性

- 構造化ログ（zerolog）、レベル分け、パネル内の「ログ」ページで確認可能；
- 指標：パネル自身 + ホスト指標を monitor モジュールが統一的に収集；
- ヘルスチェック：`/api/healthz` がプロセス/DB/ディスクの状態を返す。

---

## 11. デプロイ

### 11.1 成果物

- `opsmini` 単一バイナリ（`-s -w` 圧縮後およそ 25~30MB）、フロントエンド、SQLite、静的リソースを内蔵；
- 設定：`/etc/opsmini/config.yaml` または環境変数；
- デフォルトポート 8888、データディレクトリ `/var/lib/opsmini/`（opsmini.db）。

### 11.1.1 マルチアーキテクチャリリース（x64 + arm64）

リリースは **Linux** の **x86_64（amd64）** と **ARM64（arm64）** の 2 つの命令セットをカバーする必要があり、macOS ターゲットは開発デバッグ用にのみコンパイルします。

| ターゲットプラットフォーム | 適用シナリオ |
|----------|----------|
| `linux/amd64` | 主流の x64 サーバー（Intel/AMD、クラウドベンダーの汎用機種） |
| `linux/arm64` | ARM サーバー（Graviton、Raspberry Pi、Kunpeng、Phytium など） |
| `darwin/arm64` | Apple Silicon 開発マシン（ローカルデバッグ） |
| `darwin/amd64` | Intel Mac（開発デバッグ） |

> OpsMini は **Linux ホスト**向けで、Linux インストーラのみをリリースします。macOS ターゲットは開発デバッグ専用で、デリバリ成果物ではありません。

**重要な前提**：データ層は純 Go ドライバ `glebarez/sqlite`（基盤は `modernc.org/sqlite`）を採用し、**CGO 依存なし**。したがって `CGO_ENABLED=0` により任意のプラットフォームでワンクリックで**静的リンク**のバイナリをクロスコンパイルでき、アーキテクチャごとに C クロスツールチェーンを用意する必要がありません。

**ビルド方法**（`Makefile` 準備済み）：

```bash
make build        # 当前平台
make build-all    # 全平台交叉编译 → dist/
```

成果物の命名：`opsmini-<version>-<os>-<arch>`。バージョン番号は `-ldflags -X main.version` で注入し、実行時に `opsmini -version` で確認可能。

実測済み：4 つのターゲットプラットフォームすべてがコンパイルに成功し、`file` でアーキテクチャが正しいことを確認（ELF x86-64 / ELF aarch64 / Mach-O arm64 / Mach-O x86_64）、いずれも静的リンク。

### 11.2 サービス化

```ini
[Unit]
Description=OpsMini Agent
After=network.target

[Service]
ExecStart=/usr/local/bin/opsmini agent --config /etc/opsmini/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### 11.3 リバースプロキシ（任意）

Nginx が 80/443 → 8888 をリバースプロキシし、証明書はパネル内の Web サイトモジュールまたは外部ロードバランサが管理します。

---

## 12. 開発ロードマップ（マイルストーン）

| フェーズ | 内容 | デリバリ |
|------|------|------|
| M1 骨組み | Go プロジェクト、Gin ルーティング、SQLite、GORM マイグレーション、ログイン/ユーザー/RBAC | 動く最小パネル |
| M2 システム機能 | gopsutil 指標、プロセス/ポート/ディスク/ネットワーク、ダッシュボード+監視 | 単一マシン監視のクローズドループ |
| M3 リソース管理 | Web サイト(Nginx)、データベース、ファイル、ターミナル(pty)、cron ジョブ | 1Panel 中核に匹敵 |
| M4 コンテナ | Docker SDK：コンテナ/イメージ/ボリューム/ネットワーク | コンテナ管理 |
| M5 AI | LLM アダプタ層、コンテキスト、NL 実行、AI アシスタント | 差別化機能 |
| M6 Agent API | Agent が標準 REST API（`/agent/v1`）を公開、token 認証、コマンド許可リスト | 外部統合機能 |
| M7 仕上げ | i18n、監査、レート制限、テスト、ドキュメント | 本番対応 |

---

## 13. 確認すべき決定事項

1. **Agent API の認証強度**：デフォルトは Bearer Token。mTLS 相互証明書を導入するか（より安全、デプロイは重い）？
2. **Agent API に IP 許可リストは必要か**：デフォルト無効で Token のみ。複数マシン/公衆ネットワークのシナリオで IP 制限を加えるか？
3. **ストリーミングデータ**：ログ tail / リアルタイム指標はデフォルト REST ページングポーリング。許容できるか？（v1.0 では SSE/長接続を導入しない）
4. **複数マシンの統合管理（将来）**：後で Proxy センターを導入する場合、既存の `/agent/v1` を再利用してプル・スケジューリングを行うか、別プロトコルを設けるか？（v1.0 の範囲外、メモのみ）
