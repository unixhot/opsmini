# Dashboard & Monitoring

OpsMini has built-in metric collection and monitoring/alerting capabilities, with no need to install additional monitoring components.

## Dashboard

After login you land on the dashboard by default, which shows:

- **Resource overview**: CPU usage, memory usage, load, disk, network traffic
- **Real-time time series**: CPU / memory / disk / network curves over time (rendered by ECharts)
- **System status**: uptime, host information

The metric collector samples every 10 seconds; the in-memory ring buffer retains 24 hours, and historical data is downsampled and written to SQLite (1-minute granularity retained for 7 days).

## Monitoring

The monitoring page provides:

- **Time-series queries**: view historical metrics by time range
- **Alert rules**: customize alert thresholds and durations for CPU / memory / disk / service and other metrics
- **Alert events**: view triggered alert events and their status

### Alert Rule Example

| Field | Description |
|------|------|
| Name | Rule name, e.g. "CPU persistently high" |
| Metric | `cpu` / `mem` / `disk` / `service` |
| Condition | e.g. `>90%` |
| Duration | Time the condition must persist before triggering |
| Notification method | The associated notification channel |

## Notifications

After an alert triggers, it can be pushed to administrators via the notification center, with support for in-panel notifications and read management.

## Prometheus Integration

The panel has a built-in node_exporter-compatible `/metrics` endpoint that Prometheus can scrape directly; see [Prometheus Metrics](../api/prometheus.md) for details.
