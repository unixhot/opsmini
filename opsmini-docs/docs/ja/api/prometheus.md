# Prometheus 指標（/metrics）

OpsMini には **node_exporter 互換**の `/metrics` エンドポイントが内蔵されており、node_exporter を別途インストールする必要なく、Prometheus が直接 scrape できます。

## 有効化と認証

`config.yaml` で有効化します（デフォルトで有効）：

```yaml
metrics:
  enabled: true      # /metrics エンドポイントを有効にするか
  user: ""           # Basic 認証ユーザー名。空なら認証なし
  password: ""       # Basic 認証パスワード
```

認証情報の優先順位：パネル「設定 → 監視エクスポート」のユーザー名/パスワードが設定ファイル**より優先**されます。ユーザー名を空にすると認証なしになります。

## Prometheus 設定

```yaml
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
    basic_auth:            # 認証を有効にした場合は対応する認証情報を設定
      username: 'monitor'
      password: '<password>'
```

## 指標ファミリー

node_exporter の主要指標に合わせてあり、コミュニティの Node Dashboard をそのまま利用できます：

| 指標ファミリー | 説明 |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | 各コア・各モードの累積秒数 |
| `node_memory_MemTotal_bytes` など | メモリ Total / Free / Available / Buffers / Cached |
| `node_filesystem_size_bytes{mountpoint}` | 各マウントポイントの容量 / 空き / 使用率 |
| `node_network_receive_bytes_total{device}` | 各 NIC の送受信トラフィック |
| `node_load1` / `node_load5` / `node_load15` | ロード |
| `node_uname_info` / `node_boot_time_seconds` | ホスト情報と起動時刻 |

## 説明

- 実装は `gopsutil` が収集済みのデータを再利用し、Prometheus テキスト形式で出力します
- 単一バイナリでの配布を維持し、node_exporter プロセスは埋め込みません
- `server.secret_entry` を設定した場合、エンドポイントも同様にプレフィックスの配下に配置されます（例：`/opsmini_panel/metrics`）
