# Agent 接続（gRPC）

OpsMini のプロセスには **Agent クライアント**が組み込まれています。自ら外向きに gRPC の双方向ストリームを 1 本張り、登録・ハートビート・タスク受信・ファイル転送を行います。**相手側に SSH も受信ポートも不要**なため、NAT 配下や社内ネットワーク、グローバル IP のないホストでも使えます。

!!! warning "旧 /agent/v1 REST API について"
    `/agent/v1` REST API と「設定 → API Token」画面は **v1.2.1 で削除**しました。外部システムから操作する場合は **MCP**（`/mcp`、[パネル API](browser-api.md) 参照）を、複数ホストの統合管理には本記事の gRPC Agent を使用してください。

## 通信モデル

| 方向 | |
|------|------|
| Agent → Server | `Register` · `Heartbeat` · `TaskResult` · `FileChunkAck` |
| Server → Agent | `RegisterAck` · `ExecuteTask` · `PushFileTask` · `Ping` |

- ポートは 1 つで十分：HTTP/2 多重化により SaltStack のような 2 ポート構成は不要
- 切断時は指数バックオフで再接続し、定期的なハートビートで維持
- 同名ホストが再接続した場合、古い接続は切断されます

## 設定

```yaml
agent:
  server_addr: "47.104.172.52:4507"
  server_token: "<token from server>"
  heartbeat_seconds: 30
```

`server_addr` を空にすると Agent モードは無効になり、単体のパネルとして動作します。

## できること

| 機能 | 説明 |
|------|------|
| コマンド / スクリプト実行 | `ExecuteTask` の oneof で command と script を区別 |
| ファイル配布 | `PushFileTask` がチャンクをストリーム配信、`resume_offset` と md5 検証に対応 |
| ファイル取得 | Agent 側で読み取り、そのまま返送 |
| メトリクス収集 | gRPC ではなく `/metrics` を Prometheus が取得 |

!!! note "サーバー側のコード"
    OpsMini Server は OpsAnt プロジェクト（`github.com/opsany/opsmini-server`）にあり、本リポジトリは Agent 側（`internal/agent/client.go`）のみを含みます。
