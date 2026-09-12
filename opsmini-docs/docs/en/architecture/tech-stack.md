# Tech Stack

## Selection Overview

| Layer | Choice | Rationale |
|----|------|------|
| Language | Go 1.25+ | Single binary, cross-compilation, good concurrency, mature ecosystem |
| Web framework | Gin | Largest ecosystem, rich middleware |
| Database | SQLite | Embedded, zero-ops, fits the single-machine scenario |
| SQLite driver | `glebarez/sqlite` (pure Go) | No CGO, worry-free cross-compilation |
| ORM | GORM | High development efficiency; complex queries can fall back to raw SQL |
| Realtime communication | gorilla/websocket | Terminal, log tail, metrics push |
| Task scheduling | robfig/cron | Panel scheduled tasks |
| System monitoring | gopsutil | Cross-platform CPU / memory / disk / processes |
| Docker | Official SDK | Containers / images / volumes / networks |
| Authentication | JWT + refresh | Stateless API + optional sessions |
| 2FA | TOTP (RFC 6238) | Second-factor login verification |
| AI | Abstracted LLM interface | Unified adaptation for OpenAI / DeepSeek / Qwen / Ollama |
| Frontend | Vue 3 + ECharts | Modern SPA + charts |

## Overall Architecture

```
┌──────────────────────────────────────────────────────────┐
│              OpsMini (single-machine panel, single binary) │
│                                                          │
│   Vue3 SPA (build output embedded into the binary)        │
│        │  HTTP / WebSocket                                │
│   ┌────▼───────────────────────────────────────────────┐  │
│   │  Gin Router → Middleware (auth/permission/audit/    │  │
│   │  rate limit/security entry)                         │  │
│   │  Controller → Service → Repository                 │  │
│   └────┬──────────────────────────────────────────────┘  │
│        │                                                  │
│   ┌────▼───────────────────────────────────────────────┐  │
│   │  SQLite (business data)  │  System resource adapter │  │
│   │  users/cron/...          │  Docker SDK / crontab /  │  │
│   │                          │  gopsutil / filesystem /  │  │
│   │                          │  SSH                      │  │
│   └─────────────────────┴──────────────────────────────┘  │
│                                                          │
│   Two external REST APIs:                                 │
│   · /api/v1   Panel API (JWT + RBAC)                      │
│   · /agent/v1 Agent API (Token + command allowlist)       │
└──────────────────────────────────────────────────────────┘
```
