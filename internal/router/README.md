# internal/router

Route registration and dependency injection.

Assembles the Gin engine, wires handlers to services/repositories, and
registers:

- `/api/v1/*` — panel API (JWT + RBAC);
- `/agent/v1/*` — machine API (Agent token);
- `/terminal` — WebSocket terminal;
- `/static/*` and SPA fallback — embedded frontend.
