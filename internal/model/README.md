# internal/model

GORM data models, one file per aggregate.

| File | Purpose |
|------|---------|
| `accesslog.go` | AccessLog — panel HTTP access log entry |
| `alertevent.go` | AlertEvent — triggered alert event record |
| `alertrule.go` | AlertRule — monitoring alert rule configuration |
| `app.go` | App — software store application template |
| `appcategory.go` | AppCategory — application category |
| `auditlog.go` | AuditLog — audit log entry |
| `baselineresult.go` | BaselineResult — security baseline check result (per scan) |
| `cronjob.go` | CronJob — scheduled cron job |
| `database.go` | Database — managed database instance |
| `fim.go` | FimBaseline / FimChange — file integrity baseline & change records |
| `mcp.go` | McpServer — MCP service configuration |
| `metricpoint.go` | MetricPoint — persisted system metric sample |
| `notification.go` | Notification — notification record (with levels) |
| `role.go` | Role — panel role and permission registry |
| `setting.go` | Setting — key-value panel setting |
| `threatfinding.go` | ThreatFinding — suspicious process/cron/startup finding |
| `user.go` | User — user account (with role constants) |
| `website.go` | Website — managed website site |
