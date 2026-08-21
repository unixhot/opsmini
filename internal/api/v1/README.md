# internal/api/v1

Version 1 HTTP handlers (controllers).

Two API families are exposed:

- `/api/v1/*` — panel API for the browser UI (user JWT + RBAC);
- `/agent/v1/*` — machine API for external systems (Agent Token + command whitelist).

Files map one-to-one to backend modules:

| File | Purpose |
|------|---------|
| `accesslog.go` | Access log query endpoints |
| `agent.go` | `/agent/v1` machine endpoints (Agent Token + command whitelist) |
| `ai.go` | AI chat endpoints |
| `alert.go` | Alert rule CRUD endpoints |
| `alertevent.go` | Alert event query endpoints |
| `appcategory.go` | App store category endpoints |
| `appstore.go` | Software store app endpoints |
| `audit.go` | Audit log query endpoints |
| `auth.go` | Login / refresh / logout endpoints |
| `baseline.go` | Security baseline check endpoints |
| `cron.go` | Cron job CRUD endpoints |
| `crontab.go` | System crontab query endpoints |
| `dashboard.go` | Dashboard overview & metrics endpoints |
| `database.go` | Database instance CRUD endpoints |
| `docker.go` | Containers / images / volumes / networks endpoints |
| `docker_exec.go` | Interactive container exec (WebSocket + Docker exec) |
| `file.go` | File browsing / upload / mkdir / rename / delete |
| `fim.go` | File integrity monitoring endpoints |
| `firewall.go` | Firewall rule endpoints |
| `log.go` | Log reading endpoints |
| `loginsecurity.go` | Login security analysis endpoints |
| `mcp.go` | MCP server configuration endpoints |
| `notification.go` | Notification endpoints |
| `role.go` | Role & permission endpoints |
| `security.go` | Security overview endpoints |
| `setting.go` | Panel settings endpoints |
| `skill.go` | Skill package endpoints |
| `system.go` | Processes / ports / disks / network / host info endpoints |
| `terminal.go` | Web terminal (WebSocket + pty) |
| `threat.go` | Threat finding endpoints |
| `user.go` | User CRUD endpoints |
| `website.go` | Website CRUD endpoints |
| `*_test.go` | Unit tests (`agent_test.go`) |
