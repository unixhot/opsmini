# internal/service

Business logic layer.

Services orchestrate repositories and system resources (gopsutil, Docker
SDK, crontab, pty), defining transaction boundaries. One file per module.

| File | Purpose |
|------|---------|
| `ai.go` | LLM adapter for the AI chat assistant (tool calling, system context) |
| `alert.go` | Alert rule CRUD business logic |
| `alertmonitor.go` | Background alert evaluation loop (fires events + notifications) |
| `appcategory.go` | App store category management |
| `appstore.go` | Software store app management and one-click Docker deployment |
| `apptemplate.go` | Official app template manifest parsing |
| `audit.go` | Audit log recording |
| `auth.go` | Login, JWT token pair issuance, and MFA enforcement |
| `baseline.go` | Security baseline check execution and result aggregation |
| `cron.go` | Cron job CRUD and scheduling |
| `crontab.go` | System crontab parsing (system cron jobs) |
| `database.go` | Database instance management |
| `docker.go` | Docker container/image/volume/network operations |
| `file.go` | File operations (with path-traversal protection) |
| `fim.go` | File integrity monitoring (SHA256 baseline + change detection) |
| `firewall.go` | Firewall rule management (ufw/firewalld) |
| `log.go` | Log file reading (system/app logs) |
| `loginsecurity.go` | Login security analysis (brute force, anomalies, SSH suggestions) |
| `mcp.go` | MCP server configuration management |
| `metrics.go` | Background system metric collection and persistence |
| `notification.go` | Notification management and dispatch |
| `prometheus.go` | Prometheus text-format metric export (node_exporter compatible) |
| `role.go` | Role and permission management |
| `security.go` | Security overview scoring across security modules |
| `securitymonitor.go` | Background security monitor (FIM + threat scan orchestration) |
| `setting.go` | Panel settings business logic |
| `skill.go` | Skill package management (install/list/remove) |
| `system.go` | System info collection (host, processes, ports, disks, network) |
| `threat.go` | Threat finding detection (suspicious processes/cron/startup) |
| `totp.go` | TOTP secret generation, otpauth URL, and code validation |
| `user.go` | User CRUD and password management |
| `website.go` | Website management |
| `*_test.go` | Unit tests (`file_test.go`) |
