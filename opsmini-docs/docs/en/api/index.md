# REST API

OpsMini exposes two standard REST APIs for the browser UI and external integration.

## Overview

| API | Prefix | Consumer | Auth |
|-----|--------|----------|------|
| Panel API | `/api/v1` | Browser (humans) | User JWT + RBAC |
| Agent access | gRPC (agent dials out) | Fleet management | Server token + bidirectional stream |
| Prometheus metrics | `/metrics` | Monitoring systems | Optional Basic auth |

Unified response: `{ "code": 0, "message": "ok", "data": ... }`. A non-zero `code` is a business error.

<div class="grid cards" markdown>

-   :material-web: **[Panel API (/api/v1)](browser-api.md)**

    ---

    Auth, users, monitoring, websites, containers, files, AI, and more.

-   :material-api: **[Agent access (gRPC)](agent-api.md](agent-api.md)**

    ---

    Machine-to-machine resource management and command execution.

-   :material-chart-line: **[Prometheus /metrics](prometheus.md)**

    ---

    node_exporter-compatible metrics, scraped directly by Prometheus.

</div>
