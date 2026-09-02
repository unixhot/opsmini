# Agent API (/agent/v1)

The Agent API is a machine-to-machine REST interface for monitoring platforms, automation scripts, and orchestration tools.

## Differences from the Panel API

| Dimension | Panel API `/api/v1` | Agent API `/agent/v1` |
|-----------|---------------------|-----------------------|
| Consumer | Browser (humans) | External systems (machines) |
| Auth | User JWT + RBAC | Agent Token (Bearer) |
| Scope | Full UI + terminal WS | Resource query + command execution |
| Design | Interactive | Automation-oriented (idempotent, retryable) |

## Authentication

1. Generate an access token on the panel "Settings → API Token" page (the token persists in the database, no longer written to `config.yaml`)
2. Send `Authorization: Bearer <token>`
3. The middleware verifies the token with constant-time comparison; an empty token disables the Agent API

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/agent/v1/health` | Health check |
| GET | `/agent/v1/version` | Version info |
| GET | `/agent/v1/status` | Status summary: cpu / mem / disk / services |
| GET | `/agent/v1/system/info` | Host info (hostname / os / kernel) |
| GET | `/agent/v1/system/processes` | Process list |
| GET | `/agent/v1/system/ports` | Listening ports |
| GET | `/agent/v1/system/disks` | Disks / mounts |
| GET | `/agent/v1/websites` | Websites |
| GET | `/agent/v1/databases` | Databases |
| GET | `/agent/v1/cron-jobs` | Cron jobs |
| GET | `/agent/v1/containers` | Containers |
| POST | `/agent/v1/commands` | Execute command (allowlisted) |
| POST | `/agent/v1/script/run` | Run a script |
| POST | `/agent/v1/file/upload` | Upload a file |

## Command Execution & Allowlist

`/commands` only allows explicitly authorized command prefixes; all executions are audited. The allowlist lives in `config.yaml` under `agent.allowed_commands`:

```yaml
agent:
  allowed_commands:
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

## Example

```bash
curl -H "Authorization: Bearer <token>" \
     https://<host>:8888/agent/v1/status
```

## Security Recommendations

- Use HTTPS
- Add an IP allowlist for high-security scenarios
- Keep the command allowlist minimal
- Regenerate the token immediately on the "Settings → API Token" page if leaked
