# internal/middleware

Gin middleware.

| File | Purpose |
|------|---------|
| `accesslog.go` | HTTP access logging (records each API request) |
| `agent.go` | Agent token authentication for `/agent/v1` |
| `audit.go` | Audit logging for write operations |
| `auth.go` | JWT authentication + RBAC (role-based access control) |
| `cors.go` | Cross-Origin Resource Sharing headers |
| `metrics_auth.go` | Token authentication for the Prometheus metrics endpoint |
| `perm.go` | Permission check helper for role/perm validation |
| `ratelimit.go` | In-memory fixed-window rate limiter (login brute-force protection) |
