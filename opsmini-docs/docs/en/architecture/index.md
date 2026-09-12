# Architecture

The technical architecture and design decisions of OpsMini.

<div class="grid cards" markdown>

-   :material-sitemap-outline: **[Overall Architecture](index.md)**

    ---

    Single-machine panel, monolithic single binary, separated frontend/backend + embedded packaging.

-   :material-cube-outline: **[Tech Stack](tech-stack.md)**

    ---

    Rationale for Go · Gin · GORM · SQLite (pure Go) · Vue 3 · ECharts.

-   :material-folder-outline: **[Directory Structure](directory.md)**

    ---

    Layered architecture (Controller → Service → Repository) and directory organization.

</div>

## Core Architecture Decisions

- **Single-machine monolith**: no microservices, delivered as a single process and single binary
- **SQLite**: embedded and zero-ops, fully sufficient for the small volume of business data
- **Dual API**: `/api/v1` for the UI, `/agent/v1` for integrations, with isolated authentication and permissions
- **Pure Go without CGO**: one-click cross-compilation to a statically linked binary on any platform
