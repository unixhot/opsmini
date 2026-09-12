# Directory Structure

## Layers

Each module strictly follows three layers internally:

```
Controller (HTTP handler): parse and validate parameters, call Service, assemble response
    → Service (business logic): orchestrate Repository and system resources, transaction boundaries
    → Repository (GORM): pure data access, no business logic
```

## Directory Organization

```
opsmini/
├── cmd/
│   └── agent/main.go          # Main program entry
├── internal/
│   ├── router/                # Route registration, API versions
│   ├── middleware/            # Auth, permission, audit, rate limit, security entry
│   ├── api/v1/                # Controller (split by module)
│   ├── service/               # Service (split by module)
│   ├── repository/            # Repository (split by module)
│   ├── model/                 # GORM data models
│   ├── config/                # Configuration loading
│   └── pkg/                   # Common utilities (jwt/response/store)
├── web/                       # Vue3 frontend (embedded after build)
│   ├── index.html             # SPA entry
│   └── static/                # Static libraries such as echarts/vue/xterm
├── configs/                   # Default configuration examples
├── docs/                      # Documentation
└── install/                   # One-click install script
```

## Key Modules

| Module | Responsibility |
|------|------|
| `auth` | Login / logout, sessions, 2FA, RBAC |
| `setting` | Panel configuration, theme, menu visibility, language |
| `dashboard` / `monitor` | Metric aggregation, time-series collection, alert rules |
| `website` / `database` / `appstore` | Websites, databases, app store |
| `container` | Docker containers / images / volumes / networks |
| `system` / `file` / `terminal` | System resources, files, web terminal |
| `cron` / `log` | Scheduled tasks, logs |
| `ai` | LLM integration, context, natural-language execution |
| `security` | Baseline, FIM, threats, firewall, login security |
| `agent` | External REST API (`/agent/v1`) |
