# Quick Start

Welcome to OpsMini — a lightweight Linux server panel. This guide gets you installed, logged in, and oriented in minutes.

## Start Here

<div class="grid cards" markdown>

-   :material-rocket-launch: **[5-Minute Install](quick-install.md)**

    ---

    One command to deploy. An admin account with a random password is generated on first start.

-   :material-login: **[First Login](first-steps.md)**

    ---

    Sign in, change your password, and enable two-factor authentication (MFA).

-   :material-information-outline: **[Introduction](introduction.md)**

    ---

    Learn about OpsMini's positioning, key features, and tech stack.

</div>

## Key Concepts

| Concept | Description |
|---------|-------------|
| **Standalone panel** | Each host runs an independent OpsMini instance, no central node required |
| **Single binary** | Frontend embedded via `go:embed`; deploy by copying one executable |
| **Dual API** | `/api/v1` for the browser UI (JWT + RBAC), `/agent/v1` for machines/third parties (Token + command allowlist) |
| **AI-powered ops** | Natural-language diagnosis, log analysis, command execution via OpenAI / DeepSeek / Qwen / Ollama |

## Next Steps

After installing, read in order:

1. [Installation](../installation/index.md)
2. [Configuration](../configuration/index.md)
3. [Features](../features/index.md)
4. [REST API](../api/index.md)
