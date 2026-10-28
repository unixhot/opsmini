# Changelog

Change records for each version.

## v1.3.0 (2026-10-28)

**Autonomous Agent: from "you ask, it answers" to "it watches on its own"**

This version advances OpsMini from "an AI assistant that does work on command" to a resident Agent that "finds problems on its own, judges on its own, and acts within your boundaries." The core is a closed loop from collection to verification, not just a few more buttons.

**Perception layer (internal/sentinel)**

- Six types of collectors run continuously: log spikes (hourly-bucket EWMA baseline + 20MB/min hard threshold), login anomalies (non-working hours / first-seen IP / success after brute force / `authorized_keys` just changed), web directory script drops (14 types of WebShell signatures, strong signatures go straight to high), process spawning (web user starting shell / curl / python), suspicious outbound connections, metric alerts
- Collection results are uniformly normalized into **Signal**: fingerprint deduplication, persisted to SQLite; passed via in-process channel, no message queue introduced
- Collection frequency is tiered from 2s–20s; for anomalous logs, look up the process holding the file, avoiding "the log is growing but we don't know who is writing it"

**Event center**

- Signals are aggregated into **events** by fingerprint and time window; an event carries title, category, level, object, and occurrence count
- Six filters (pending / pending wake / all / handled / failed / ignored) all carry counts; selecting one shows the definition for that status
- Event details can drill down to related signals, Agent run records, and the per-step action chain (tool, parameters, verdict, raw output)

**Policy engine (internal/trigger)**

- Five built-in policies work out of the box: `webshell-respond`, `auth-anomaly`, `proc-anomaly` (critical only), `log-growth` (shadow mode), `net-anomaly` (disabled by default)
- The Agent is woken only when three conditions are met simultaneously: category matches, level reaches threshold, and not in cooldown; otherwise only the event is recorded, **without consuming model quota**
- Each policy can be configured with its own budget: write operation count, model steps, wall-clock duration, token limit; stop when any is exceeded
- Policies are data, not code: add, delete, modify, adjust thresholds, and switch modes in the UI without restarting

**Three run modes**

- `shadow` observe only: gather evidence as usual, write conclusions and "what it plans to do," execute no write operations; suitable for running for a few days to calibrate judgment first
- `assist` assist: read-only actions execute automatically, any write action stops and waits for human confirmation — the recommended daily setting
- `auto` autonomous: only low-risk actions in the `auto_tools` whitelist skip confirmation; L2 and above always require human approval

**Making irreversible actions reversible**

- Delete → **quarantine**: move files to the quarantine area, record original path, checksum, and permissions, restore with one click
- `kill -9` → **SIGSTOP freeze**: stop first to preserve the scene, decide after confirmation
- Permanent ban → **temporary ban with TTL**: automatically unbanned on expiry, a mistaken ban won't become an incident
- **Snapshot** before acting: keep a backup before changing config, roll back if verification fails

**Five safety gates (internal/safety)**

- Parameter policy, write budget, protected paths, killswitch, post-action assertion verification (`no_new_signal`, `port_listen`, `file_absent`, etc.)
- Events that fail verification are marked as failed and handed back to a human, rather than glossed over as success; empty reports also count as failure
- Every tool call goes into the action audit: who triggered it, what was called, what parameters were passed, what was returned

**Data retention**

- A new "Data Retention" page in panel settings: events and signals retained 30 days by default, action audit 180 days, quarantine items and ban records 30 days
- Pending events together with their signals and action chains are **never automatically deleted**; quarantined files are untouched until restored, bans untouched until unbanned
- Also added "Clear event history": clear everything at once regardless of retention window (including quarantined file copies and ban records, removing firewall rules, resetting policy cooldowns), executed after second confirmation

**Drill and verification**

- Added `scripts/chaos-drill.sh`: inject four types of faults (log spike / WebShell drop / SSH brute-force success / web user spawning shell), then use the API to verify whether the perception layer produces corresponding events
- All four scenarios passed on a public network test machine; in `auto` mode, a real WebShell was quarantined and verified successfully

**Other improvements**

- Login page copy changed to Agent positioning (7 languages), tab title follows login status
- Notification cards can be closed by clicking blank space on the page
- Build version stamp switched to `git describe --tags --dirty`, added `make release-check` release pre-check to avoid releasing with a dirty working tree

**Upgrade notes**

- v1.2.1 → v1.3.0: just replace the binary and restart; new tables are created automatically, no migration needed
- After upgrading, it is recommended to first look at the five built-in policies in "Event Center → Policies": by default they are all `assist` or `shadow`, and will not automatically change the system

## v1.2.1 (2026-10-27)

**Fix: remove four multi-host AI tools that crossed an architectural boundary**

v1.2.0 registered four multi-host tools (`list_remote_hosts` / `remote_exec` / `remote_pull_file` / `remote_push_file`) that let the panel's AI dispatch commands and files to other enrolled hosts through the OpsMini Server control-plane HTTP API on port 4508. That implementation crossed a boundary and has been removed:

- 4508 is an **internal** API the OpsMini Server offers to OpsAnt. It is scoped to "local host / internal network" and performs no authentication.
- On the multi-host path the panel is the **managed Agent**; it should talk to the Server only over the gRPC connection on 4507 (register, heartbeat, receive tasks) and must not call the control plane back.
- Making that design work would require exposing 4508 to the internet — i.e. publishing an unauthenticated remote command entry point.

The tool set therefore returns to **29** (16 read-only / 5 low-risk write / 8 high-risk write). Multi-host orchestration and multi-host AI belong to OpsAnt, which already provides a unified execution API (`/api/opsant/v1/tasks/run-shell/`, `tasks/run-script/`, `tasks/send-file/`, `tasks/get-minion-file/`, ...) plus the salt / ansible / opsmini engines.

**No migration needed**: those four tools required 4508 to be reachable and were never actually usable.

## v1.2.0 (2026-10-27)

**AI agent: from advisor to operator**

- **Self-built lightweight ReAct kernel**: a Reason → Act → Observe loop with no agent framework, keeping the single binary and CGO-free build intact; one turn is capped at 12 steps, after which the agent stops calling tools and reports what it already knows
- **33 built-in tools**: 16 read-only (system overview, processes, ports, disks, directory usage, log search, services, containers, security, ...), 5 low-risk writes, 8 high-risk writes, plus 4 multi-machine tools registered dynamically from the Agent configuration
- **Streaming tool calls**: `tool_calls` fragments are accumulated and stitched per index; vendors that cannot stream tool calls fall back to a non-streaming retry automatically
- **Visible thinking chain**: the UI shows step by step which tool ran, with what arguments, and its raw output, so the conclusion is auditable

**Tiered authorisation and human-in-the-loop**

- **Four-level risk model**: L0 read-only runs freely / L1 low-risk asks by default (an administrator may opt a tool into unattended execution) / L2 high-risk always asks and can never be exempted / L3 catastrophic operations are hard-blocked by pattern
- **L3 blocks before execution**: `rm -rf /`, `mkfs`, `dd of=/dev/sd*`, fork bombs, flushing the firewall, overwriting `/etc/passwd` and the like are refused even if an administrator approves them; two extra rules protect the panel itself (never touch the data directory, never close the panel port)
- **Suspend and resume**: a high-risk call is persisted and suspended; approving or rejecting it in the chat resumes the message sequence from SQLite. The rejection path was verified to leave no side effects

**Bidirectional MCP**

- **MCP client**: built on the official MCP Go SDK with both stdio and Streamable HTTP transports; tools of an external MCP server are exposed as `mcp__<server>__<tool>` and the configuration is reconciled every 2 minutes
- **MCP server**: publishes its own toolset on `/mcp` behind a Bearer token, exposing only L0/L1 tools by default (L2 is withheld from unauthorised clients because an external client cannot render the panel's approval dialog)

**Scheduled AI tasks**

- **A fourth scheduled-task kind, "Agent task"**: write a prompt and it runs on its cron schedule; scheduling, enable/disable, last-run bookkeeping and failure alerts are all reused from the existing machinery
- **Authorisation degrades when unattended**: nobody is around to click "Approve", so any tool mapping to L2 is refused outright and the reason is fed back to the model, which switches to read-only tools and states in the report that the operation needs a human, along with the exact command
- **Reliability and cost control**: per-task step budget and wall-clock timeout; three consecutive failures disable the task automatically, so a broken job cannot alert forever; saving the task clears the failure count
- **Five built-in inspection presets**: host inspection, security inspection, disk usage, service status, log anomalies, plus four common schedule presets

**Single-file HTML reports**

- **A report is one self-contained HTML file**: inline styles, no script, no external stylesheet, no web font, no images — it renders from a USB stick, as a mail attachment or from any static host
- **Open or download**: each task row offers "Open" (rendered directly in a new tab) and "Download"; Chinese file names are RFC 5987 encoded
- **Archived under the data directory**: both `.html` and `.md` land in `<data dir>/reports/<task name>/`, one subdirectory per feature for all generated artifacts
- **Safe rendering**: the body mixes model prose with host data (log excerpts, process command lines, config values); the renderer escapes the whole document before applying inline rules and only allows http/https/mailto targets, so a `<script>` in a log line is displayed as text. A strict CSP is sent as a second line of defence
- **When you need PDF**: use the browser's "Print → Save as PDF" on the open report; the stylesheet already carries print rules

**Notifications and audit**

- **Shared notifier**: notification dispatch was extracted from alerts into one `Notifier` (in-panel / email / WeCom / DingTalk / Feishu), so AI tasks and alerts behave identically; an empty channel list defaults to the in-panel bell
- **Audit trail**: AI tool calls are written to the audit log; refusals during unattended runs are recorded as `unattended-deny`, human rejections with a `reject:` prefix

**Other improvements**

- The sidebar version label now shows the real backend version instead of a hard-coded string
- Added the AI agent design document `docs/ai-agent.md` in 7 languages
- Administrators can tune per-tool policy under "AI Assistant → Tool permissions"; high-risk tools stay locked to "Ask first"

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
