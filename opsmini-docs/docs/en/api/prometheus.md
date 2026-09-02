# Prometheus Metrics (/metrics)

OpsMini ships a **node_exporter-compatible** `/metrics` endpoint — no extra node_exporter needed.

## Enable & Auth

Configure in `config.yaml` (enabled by default):

```yaml
metrics:
  enabled: true      # enable /metrics endpoint
  user: ""           # Basic auth username; empty = no auth
  password: ""       # Basic auth password
```

Credential priority: the username/password in panel "Settings → Metrics Export" **override** the config file; an empty username means no auth.

## Prometheus Configuration

```yaml
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
    basic_auth:            # required when auth is enabled
      username: 'monitor'
      password: '<password>'
```

## Metric Families

Aligned with node_exporter core metrics, ready for community Node dashboards:

| Family | Description |
|--------|-------------|
| `node_cpu_seconds_total{cpu,mode}` | Cumulative seconds per core/mode |
| `node_memory_MemTotal_bytes` etc. | Memory Total / Free / Available / Buffers / Cached |
| `node_filesystem_size_bytes{mountpoint}` | Filesystem capacity / free / usage |
| `node_network_receive_bytes_total{device}` | Per-interface traffic |
| `node_load1` / `node_load5` / `node_load15` | Load average |
| `node_uname_info` / `node_boot_time_seconds` | Host info and boot time |

## Notes

- Reuses data already collected by `gopsutil`, output in Prometheus text format.
- Keeps the single-binary delivery; no embedded node_exporter process.
- With `server.secret_entry` set, the endpoint also mounts under the prefix (e.g. `/opsmini_panel/metrics`).
