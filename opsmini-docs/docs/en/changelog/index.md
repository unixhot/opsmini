# Changelog

Change records for each version.

## v1.0.0 (current · first official release)

The first official release of OpsMini, delivered as a single binary, with core capabilities:

- **AI operations**: natural-language diagnostics, intelligent log analysis, command generation and execution (SSE streaming output), MCP tool integration
- **Monitoring and alerting**: time-series collection of CPU / memory / disk / network metrics, with alert rules supporting email, panel, WeCom, DingTalk, and Feishu group-bot notifications
- **Host security**: security score, baseline checks, File Integrity Monitoring (FIM), threat detection, firewall, login security
- **App store**: app categories + one-click install + official template sync
- **AI skills (Skill Hub)**: skill search, install, and upload
- **Web terminal**: WebSocket + pty interactive shell, multiple sessions
- **Routine operations**: websites, databases, scheduled tasks, file management
- **Two-factor authentication (2FA)**: global toggle + per-user TOTP binding
- **Monitoring export**: Prometheus `/metrics` endpoint (node_exporter compatible)
- **Notification management**: SMTP / WeCom / DingTalk / Feishu
- **Streamlined configuration**: JWT secret, Agent Token, and AI configuration are all managed in the panel; the configuration file keeps only minimal items
- **Logging system**: configurable log level (info / warn / error), logs written to disk
- **Account recovery**: `-reset-pass` / `-reset-mfa` subcommands
- **7-language i18n**: Simplified Chinese / Traditional Chinese / English / Japanese / Korean / Thai / German
- **White minimalist UI**: white + black text color scheme, accented with brand blue
- **Dual API**: `/api/v1` (for browsers) + `/agent/v1` (standard REST between machines)

> The complete version history is based on the repository Releases: [GitHub Releases](https://github.com/unixhot/opsmini/releases)
