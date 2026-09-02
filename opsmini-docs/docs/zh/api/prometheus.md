# Prometheus 指标（/metrics）

OpsMini 内置 **node_exporter 兼容**的 `/metrics` 端点，无需额外安装 node_exporter，Prometheus 可直接 scrape。

## 启用与认证

在 `config.yaml` 中启用（默认已启用）：

```yaml
metrics:
  enabled: true      # 是否启用 /metrics 端点
  user: ""           # Basic 认证用户名，空则无认证
  password: ""       # Basic 认证密码
```

认证凭据优先级：面板「设置 → 监控导出」中的用户名/密码 **优先于** 配置文件；用户名留空即无认证。

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

## 指标族

已对齐 node_exporter 的核心指标，可直接套用社区 Node Dashboard：

| 指标族 | 说明 |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | 每核各模式累计秒数 |
| `node_memory_MemTotal_bytes` 等 | 内存 Total / Free / Available / Buffers / Cached |
| `node_filesystem_size_bytes{mountpoint}` | 各挂载点容量 / 可用 / 使用率 |
| `node_network_receive_bytes_total{device}` | 各网卡收发流量 |
| `node_load1` / `node_load5` / `node_load15` | 负载 |
| `node_uname_info` / `node_boot_time_seconds` | 主机信息与启动时间 |

## 说明

- 实现复用 `gopsutil` 已采集的数据，按 Prometheus 文本格式输出
- 保持单二进制交付，不嵌入 node_exporter 进程
- 若配置了 `server.secret_entry`，端点同样挂在前缀下（如 `/opsmini_panel/metrics`）
