# Agent API（/agent/v1）

Agent API は**マシン / サードパーティシステム**向けの標準 REST インターフェースで、監視プラットフォーム、自動化スクリプト、オーケストレーションツールが連携・呼び出しできるようにするものです。

## パネル API との違い

| 観点 | パネル API `/api/v1` | Agent API `/agent/v1` |
|------|-------------------|----------------------|
| 利用者 | ブラウザ（人） | 外部システム（マシン） |
| 認証 | ユーザーセッション JWT + RBAC | Agent Token（Bearer） |
| 範囲 | 全 UI 機能 + ターミナル WS | リソース照会 + コマンド実行（UI / ターミナルなし） |
| 設計 | 対話向け | 自動化向け（冪等・再試行可能） |

## 認証

1. パネルの「設定 → API Token」ページでアクセストークンを生成（トークンはデータベースに永続化され、`config.yaml` には記載されません）
2. リクエストに `Authorization: Bearer <token>` を付与
3. ミドルウェアが定数時間比較でトークンを検証。トークンを空にすると Agent API は無効化されます

## エンドポイント一覧

| メソッド | パス | 説明 |
|------|------|------|
| GET | `/agent/v1/health` | ヘルスチェック（死活監視） |
| GET | `/agent/v1/version` | バージョン情報 |
| GET | `/agent/v1/status` | ステータスサマリー：cpu / mem / ディスク / 稼働中サービス |
| GET | `/agent/v1/system/info` | ホスト情報（hostname / os / カーネル） |
| GET | `/agent/v1/system/processes` | プロセス一覧 |
| GET | `/agent/v1/system/ports` | ポート監視 |
| GET | `/agent/v1/system/disks` | ディスク / マウントポイント |
| GET | `/agent/v1/websites` | ウェブサイト一覧 |
| GET | `/agent/v1/databases` | データベース一覧 |
| GET | `/agent/v1/cron-jobs` | スケジュールタスク |
| GET | `/agent/v1/containers` | コンテナ一覧 |
| POST | `/agent/v1/commands` | コマンド実行（ホワイトリスト） |
| POST | `/agent/v1/script/run` | スクリプト実行 |
| POST | `/agent/v1/file/upload` | ファイルアップロード |

## コマンド実行とホワイトリスト

`/commands` は明示的に許可されたコマンドプレフィックスのみ実行でき、すべて監査に記録されます。ホワイトリストは `config.yaml` の `agent.allowed_commands` で管理します：

```yaml
agent:
  allowed_commands:
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

## 呼び出し例

```bash
curl -H "Authorization: Bearer <token>" \
     https://<host>:8888/agent/v1/status
```

## セキュリティ推奨事項

- HTTPS 転送を使用する
- 高セキュリティが求められる場面では IP ホワイトリストで送信元を制限する
- コマンドホワイトリストを最小化し、必要なコマンドのみ許可する
- トークンが漏洩したら、直ちにパネルの「設定 → API Token」で再生成する
