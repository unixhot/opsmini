# internal/repository

Data access layer (GORM).

Each repository encapsulates queries for one aggregate. No business logic
lives here — services orchestrate repositories.

| File | Purpose |
|------|---------|
| `accesslog.go` | AccessLog repository |
| `alertevent.go` | AlertEvent repository |
| `alertrule.go` | AlertRule repository |
| `app.go` | App repository |
| `appcategory.go` | AppCategory repository |
| `auditlog.go` | AuditLog repository |
| `baselineresult.go` | BaselineResult repository |
| `cronjob.go` | CronJob repository |
| `database.go` | Database repository |
| `fim.go` | FimBaseline / FimChange repositories |
| `mcp.go` | McpServer repository |
| `notification.go` | Notification repository |
| `role.go` | Role repository |
| `setting.go` | Setting repository |
| `threatfinding.go` | ThreatFinding repository |
| `user.go` | User repository |
| `website.go` | Website repository |
