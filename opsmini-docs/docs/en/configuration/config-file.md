# Main Config File

OpsMini uses a YAML config file, default `configs/config.yaml`, overridable via `-config`. If the file is missing, the program starts with built-in defaults.

The shipped example has been trimmed to a **minimal runnable config**; runtime settings (AI, Agent token, metrics auth, etc.) have moved to the panel "Settings" page.

```yaml
# OpsMini Agent config file
server:
  host: "0.0.0.0"        # listen address
  port: 8888              # listen port
  secret_entry: ""        # secret entry path prefix, e.g. /opsmini_panel (empty disables)

database:
  path: "opsmini.db"      # SQLite data file path

jwt:
  access_ttl_seconds: 900     # access token TTL (15 min)
  refresh_ttl_seconds: 604800 # refresh token TTL (7 days)

log:
  level: error               # log level: info / warn / error
  path: ""                   # log file path; empty logs to stdout (systemd journal)
```

## Options Reference

### server

| Key | Description | Default |
|-----|-------------|---------|
| `host` | Listen address; `0.0.0.0` = all interfaces | `0.0.0.0` |
| `port` | Listen port | `8888` |
| `secret_entry` | Secret entry path prefix, e.g. `/opsmini_panel`; empty disables | empty |

With `secret_entry` set, the panel and all APIs mount under the prefix (e.g. `http://<host>:8888/opsmini_panel`), letting you hide the real entry behind a reverse proxy and resist port scanning.

### database

| Key | Description | Default |
|-----|-------------|---------|
| `path` | SQLite data file path | `opsmini.db` |

### jwt

| Key | Description | Default |
|-----|-------------|---------|
| `access_ttl_seconds` | Access token TTL (seconds) | `900` |
| `refresh_ttl_seconds` | Refresh token TTL (seconds) | `604800` |

> The JWT signing secret is **no longer** configured here. On first start a 32-byte random secret is generated and persisted to the database — never written to the config file or shown in the UI.

### log

| Key | Description | Default |
|-----|-------------|---------|
| `level` | Log level: `info` / `warn` / `error` | `error` |
| `path` | Log file path; empty logs to stdout (systemd journal) | empty |

The level controls verbosity: `error` logs only errors (recommended for production, avoids SQL query log spam); `warn` additionally logs slow queries and warnings; `info` logs everything (including SQL queries, for troubleshooting). The one-click installer defaults to `/data/opsmini/opsmini.log`.

## Optional Sections

The following fields remain supported by the config struct but are omitted from the shipped example (built-in defaults apply). Declare them explicitly when needed.

### agent (command allowlist)

```yaml
agent:
  allowed_commands:        # Agent API command allowlist prefixes
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

| Key | Description | Default |
|-----|-------------|---------|
| `allowed_commands` | Command prefixes allowed by `/agent/v1/commands` | see above |

> `agent.token` is deprecated. The Agent API token is now generated and managed on the panel "Settings → API Token" page; leaving it empty disables the Agent API.

### metrics (Prometheus)

```yaml
metrics:
  enabled: true     # enable /metrics endpoint
  user: ""          # Basic auth username; empty = no auth
  password: ""      # Basic auth password
```

| Key | Description | Default |
|-----|-------------|---------|
| `enabled` | Enable the `/metrics` endpoint | `true` |
| `user` | HTTP Basic auth username; empty = no auth | empty |
| `password` | HTTP Basic auth password | empty |

> Credential priority: the username/password in panel "Settings → Metrics Export" **override** these values. An empty username means open access.

## Settings Moved to the Panel

These options have been moved out of `config.yaml` and are managed on the panel "Settings" page (stored in SQLite, applied at runtime):

| Former config | Now managed at | Notes |
|---------------|----------------|-------|
| `jwt.secret` | auto-generated (no management) | random secret persisted on first start |
| `ai.*` | Settings → AI Model Integration | model / API key / base URL / enable switch |
| `agent.token` | Settings → API Token | Agent API token; empty disables |
| `metrics.user/password` | Settings → Metrics Export | runtime override of config values |

See [Panel Settings](panel-settings.md).

## Environment Variables

| Variable | Description |
|----------|-------------|
| `OPSMINI_AI_KEY` | AI API key, **highest priority** (over panel settings and config, keeps secrets off disk) |

## Command-line Flags

| Flag | Description |
|------|-------------|
| `-config <path>` | Config file path, default `configs/config.yaml` |
| `-version` | Print version info and exit |
| `-reset-mfa <username>` | Reset a user's MFA binding (when the authenticator is lost), then exit |
| `-reset-pass <username>` | Reset a user's password to a random strong one and print it, then exit |

> `-reset-mfa` / `-reset-pass` operate on the database directly and do not start the server — used for account recovery.
