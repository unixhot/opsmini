# Prometheus 指標（/metrics）

OpsMini 內建 **node_exporter 相容**的 `/metrics` 端點，無需額外安裝 node_exporter，Prometheus 可直接 scrape。

## 啟用與認證

在 `config.yaml` 中啟用（預設已啟用）：

```yaml
metrics:
  enabled: true      # 是否启用 /metrics 端点
  user: ""           # Basic 认证用户名，空则无认证
  password: ""       # Basic 认证密码
```

認證憑據優先順序：面板「設定 → 監控匯出」中的使用者名稱/密碼 **優先於** 配置檔案；使用者名稱留空即無認證。

## Prometheus 配置

```yaml
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
    basic_auth:            # 若启用了认证，需配置对应凭据
      username: 'monitor'
      password: '<password>'
```

## 指標族

已對齊 node_exporter 的核心指標，可直接套用社群 Node Dashboard：

| 指標族 | 說明 |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | 每核各模式累計秒數 |
| `node_memory_MemTotal_bytes` 等 | 記憶體 Total / Free / Available / Buffers / Cached |
| `node_filesystem_size_bytes{mountpoint}` | 各掛載點容量 / 可用 / 使用率 |
| `node_network_receive_bytes_total{device}` | 各網絡卡收發流量 |
| `node_load1` / `node_load5` / `node_load15` | 負載 |
| `node_uname_info` / `node_boot_time_seconds` | 主機資訊與啟動時間 |

## 說明

- 實現複用 `gopsutil` 已採集的資料，按 Prometheus 文字格式輸出
- 保持單二進位制交付，不嵌入 node_exporter 程序
- 若配置了 `server.secret_entry`，端點同樣掛在字首下（如 `/opsmini_panel/metrics`）
