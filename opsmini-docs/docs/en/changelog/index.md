# Changelog

Change records for each version.

## v1.1.0 (2026-09-21)

**Multi-machine management (Agent / Server architecture)**

- **Agent gRPC client**: the Agent actively connects to OpsMini Server (single-port gRPC bidirectional stream), with heartbeat + exponential backoff reconnection; it receives execution / file tasks and reports results back
- **OpsMini Server side**: registration / heartbeat / connection management, execution task dispatch, and result reception (code migrated to the OpsAnt project as a standalone Go sub-project `opsmini-server/`; OpsMini retains only the Agent)
- **Command / script execution**: command and shell / python scripts, with results (stdout / stderr / exit code / elapsed time) reported back
- **File pull (PullFile)**: the Server pulls files from the Agent (chunked upload + size / md5 verification)
- **File push (PushFile)**: the Server pushes files to the Agent (chunking + resumable transfer + md5 verification), supporting a directory as the target (original filename preserved)
- **Connection status detection + long-lived connection keepalive**: Agent online / offline status, heartbeat timeout detection, gRPC keepalive
- **OpsAnt integration**: visually configure the OpsAnt address / Token on the panel "Settings" page (hot-swappable at runtime; the Token field supports an eye icon to toggle between plaintext and masked), SSO authentication-free redirect (enabled by default)
- **Removed the `/agent/v1` REST API**: switched to gRPC bidirectional streaming (the original REST API and the frontend "API Token" settings page were removed together)

**Panel feature enhancements**

- **Directory download on the file page**: one-click compression of a directory into a zip for download (dialog prompt + directory size + disk usage of the containing partition)
- **Directory deletion on the file page**: recursively delete directories (danger-operation dialog + secondary confirmation by typing the directory name)
- **Long-name optimization on the file page**: overlong file / directory names are truncated with an ellipsis, and hovering shows the full name (eliminates the horizontal scrollbar)
- **"Network" routing table in System Management**: added a routing table display below the network interfaces (destination network / gateway / subnet mask / flags / interface / metric)
- **MCP configuration enhancements**: supports both "Configuration file (JSON)" and "Table" input methods; SSE / HTTP methods support custom request headers (multiple) and a timeout
- **Firewall enhancements**: automatically allow the panel port and the SSH port (22) before enabling, preventing loss of access after enabling; recognizes ufw that is "installed but not enabled"
- **Prometheus metrics enhancements**: added `opsmini_cpu_usage_percent` / `opsmini_mem_usage_percent` real-time usage rates and the `node_procs_total` total process count
- **Unified confirmation dialog**: all deletion / dangerous operations now use the platform's built-in confirmation dialog (replacing the browser's native confirm)

**Terminal optimizations**

- **Output padding**: added left / right / bottom padding to command output so it no longer touches the edges
- **Fullscreen Esc fix**: pressing Esc in fullscreen mode no longer exits fullscreen (avoids conflict with Esc in vim editing mode)
- **Session persistence**: the terminal session is no longer reset after switching pages (kept within the session validity period)
- **Character set setting**: added character set selection (UTF-8 / GBK / GB18030 / Big5) to fix garbled Chinese text

**Experience optimizations**

- **Page title**: displays "OpsMini · access address" so multiple tabs can be distinguished
- **Password visibility**: added a "show / hide" eye icon to input fields such as password change and the OpsAnt integration Token
- **Settings page layout**: "Two-factor authentication" and "OpsAnt integration" are displayed side by side; the "Auto update" toggle was removed

## v1.0.0 (2026-09-13)

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
