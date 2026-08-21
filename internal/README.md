# internal

Private application code. Packages under `internal/` follow Go's internal
package convention and are not importable by external modules.

| Directory | Responsibility |
|-----------|----------------|
| [`api/`](api/README.md) | HTTP handlers (controllers), versioned |
| [`config/`](config/README.md) | Configuration loading |
| [`middleware/`](middleware/README.md) | Gin middleware (auth, audit, rate-limit, CORS) |
| [`model/`](model/README.md) | GORM data models |
| [`pkg/`](pkg/README.md) | Reusable internal packages |
| [`repository/`](repository/README.md) | Data access layer (GORM) |
| [`router/`](router/README.md) | Route registration & dependency injection |
| [`service/`](service/README.md) | Business logic layer |
