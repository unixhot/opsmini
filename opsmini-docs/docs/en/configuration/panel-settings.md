# Panel Settings

The panel "Settings" page centralizes runtime configuration. Changes take effect immediately and persist to the SQLite database — no config-file edits or restarts required. Options that previously lived in `config.yaml` (AI, Agent token, metrics auth) have moved here.

> Accessing "Panel Settings" requires the `settings.view` permission; saving requires `settings.edit` (granted to `admin` by default).

## Basic Settings

| Item | Description |
|------|-------------|
| Panel Port | Panel listen port (maps to `server.port`) |
| Panel Domain | Panel access domain, used for links and reverse-proxy setups |
| Security Entry | Hidden path prefix (maps to `server.secret_entry`), e.g. `/opsmini_panel`; empty disables |
| Enable HTTPS | Whether to enable HTTPS (recommended to terminate TLS at the reverse proxy) |

## Two-Factor (2FA)

- **Global switch**: once enabled, every account with a bound authenticator must enter a 6-digit code at login
- Each user can independently bind/unbind a TOTP authenticator (Google Authenticator / 1Password / cloud-vendor authenticator apps)
- If the authenticator is lost, run `opsmini -reset-mfa <username>` on the server to reset it

## AI Model Integration {: #ai-config }

Configure the built-in AI assistant's model access here, replacing the former `config.yaml` `ai` section:

| Item | Description |
|------|-------------|
| Enable AI Assistant | Master switch; disables AI features when off |
| Model Provider | `openai` / `deepseek` / `qwen` / `ollama` |
| Model | Model name, e.g. `gpt-4o`, `deepseek-chat`, `qwen-plus` |
| API Key | Provider key; may be left empty to read env var `OPSMINI_AI_KEY` |
| API Endpoint | OpenAI-compatible base URL, e.g. `https://api.openai.com/v1` |
| Natural-language execution | Allow AI to perform ops actions |
| Log analysis | Enable AI log analysis |
| Alert diagnosis | Enable AI alert diagnosis |

> Priority: env var `OPSMINI_AI_KEY` > panel settings > leftover `ai.*` in the config file.

## API Token

Generate an access token for external systems (monitoring platforms, automation scripts, orchestration tools):

- The token is used to call `/agent/v1`, sent as `Authorization: Bearer <token>`
- Leaving it empty **disables** the Agent API
- "Generate" creates a strong random token; it takes effect after saving
- Regenerate immediately if leaked

The command allowlist is still maintained in `config.yaml` under `agent.allowed_commands`. See [Agent API](../api/agent-api.md).

## Appearance & Theme

- **Custom accent color**: set the panel brand color (corporate brand / personal preference); default OpsMini blue `#4f6ef7`
- **Menu display / Language**: panel UI language (7 languages) and menu visibility

## Metrics Export (Prometheus)

Controls authentication for the `/metrics` endpoint (node_exporter-compatible):

| Item | Description |
|------|-------------|
| Enable auth | Turn on HTTP Basic auth for `/metrics` |
| Username / Password | Basic auth credentials; empty username means open access |

> These override `config.yaml` `metrics.user/password`. Prometheus must configure matching `basic_auth`. See [Prometheus Metrics](../api/prometheus.md).

## App Templates (App Store)

Manage app-store templates:

- **Sync official templates**: one-click sync of official templates from opsmini.com
- **Import / export**: import templates downloaded from the website for offline environments, or export local templates for backup
- **Data directory**: persistent app data directory (referenced by install scripts via the `DATA_DIR` env var)

## Notifications

Configure alert notification channels:

| Channel | Description |
|---------|-------------|
| SMTP Email | SMTP server / port / sender / recipient / username / password |
| WeCom | Group robot Webhook URL |
| DingTalk | Group robot Webhook URL |
| Feishu | Group robot Webhook URL |

Alerts are sent through the configured channels when triggered.

## Log Retention

- **Panel log retention**: retention days for audit and access logs (default 7 days), auto-cleaned when expired
- Alert event retention can be set separately (default 30 days)

## Settings Storage

Settings are stored as key-value pairs in the SQLite `settings` table and read at runtime. Two kinds of sensitive items are server-protected:

- **Read-sensitive** (`jwt_secret`, `metrics_pass`): never returned to the frontend
- **Write-protected** (`jwt_secret`): the client cannot overwrite; auto-generated and managed by the server
