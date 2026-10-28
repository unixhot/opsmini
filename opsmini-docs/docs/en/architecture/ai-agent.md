# AI Agent Design

> Milestones M8–M10 ｜ Code: `internal/aiagent/`
>
> How to use: AI Assistant → configure a model → just ask in natural language

In M8 the OpsMini AI assistant grew from "an ops advisor that can read a live
system snapshot" into **an agent that can actually operate the host**: the model
plans, calls tools, observes the results and iterates until it reaches a
conclusion — while every high-risk action still needs explicit human approval.

## 1. Why a self-built kernel instead of an Agent framework

Mature Go agent frameworks exist (Google ADK Go, CloudWeGo Eino). OpsMini still
builds its own ~1,500-line kernel, because:

1. **The moat is domain capability, not orchestration.** Frameworks solve the
   generic model + tool loop; OpsMini's value is its 29 domain tools, its graded
   safety rails and its gRPC multi-host channel, none of which a framework provides.
2. **The glue costs about as much as the kernel.** The tools are plain local Go
   functions; adapting them to a framework's tool interface is comparable work.
3. **Single binary, no CGO.** Pulling in ADK drags OpenTelemetry / genai / GCS
   into the dependency tree.
4. **Use a protocol as the extension point, not a framework.** Tools are declared
   with **MCP-compatible JSON Schema**, never a framework-private format. As a
   bonus the same registry can be republished as an MCP server.

So: **protocol-based tools + self-built kernel.** If multi-agent collaboration or
visual workflow orchestration is ever needed, switching to Eino ADK or Google ADK
Go stays possible, with OpsMini interoperating over MCP / A2A.

## 2. Kernel layout

```
internal/aiagent/
├── types.go      Tool / Result / Message / ToolCall / RiskLevel
├── registry.go   Thread-safe registry (OpenAI + MCP definitions)
├── llm.go        OpenAI-compatible client (streaming tool-call accumulation)
├── loop.go       ReAct kernel: reason → act → observe, with suspend/resume
├── guard.go      L3 hard-forbidden patterns, panel self-protection, risk→policy
├── hitl.go       Persisted, expiring human approvals
├── memory.go     Conversation persistence and context compression
├── mcpclient.go  MCP client (official Go SDK, stdio + Streamable HTTP)
├── mcpserver.go  MCP server (publishes the tool set to external clients)
├── events.go     SSE event model
└── tools/        The built-in tool set
```

## 3. The ReAct loop

One user turn maps to one `Agent.Send`:

1. Load the conversation transcript from SQLite, prepend the system prompt and a
   live host snapshot.
2. Call the model with **streaming**; text deltas are pushed to the frontend as
   `thinking` events.
3. When the model requests tools:
   - run the **L3 hard-forbidden scan** first — a hit is refused and fed back;
   - then apply the risk level and the administrator's per-tool policy:
     run immediately / require approval / disabled;
   - feed the observation back as a `tool` message and continue.
4. When the model stops requesting tools, emit the final answer.

The step budget defaults to 12 per turn; past it the agent stops calling tools
and reports what it already knows, so a run cannot run away.

## 4. Risk levels and guard rails

| Level | Name | Behaviour |
|-------|------|-----------|
| L0 | `RiskReadonly` | Read-only, runs unattended |
| L1 | `RiskLowWrite` | Low-risk write; **still confirmed by default**, an administrator may opt a specific tool into unattended execution |
| L2 | `RiskHighWrite` | High risk; **every run needs human approval** and can never be set to auto |
| L3 | Pattern based | Blocked before execution, even with approval |

L3 covers `rm -rf /` (including critical system directories), `mkfs`,
`dd of=/dev/sd*`, redirects onto block devices, shutdown/reboot,
`chmod -R 777 /`, `chown -R … /`, fork bombs, flushing iptables/nft rules,
overwriting `/etc/passwd`-style auth files, wiping shell history, `rmmod` and
`kill -9 1`.

Two extra rails protect the panel itself:

- anything that would delete or overwrite the panel data directory (binary,
  SQLite database, skills) is refused;
- firewall rules that would cut off the panel's listening port are rejected.

> Product judgement: no unattended root agent. The target shape is
> **"autonomous planning + graded authorisation"** — the AI investigates and
> advises, a human authorises anything destructive.

## 5. Human-in-the-loop

When a call needs approval the kernel writes an `ai_pending_approvals` record
(session, tool, arguments, risk, preview text, expiry — 10 minutes by default),
pushes a `confirm` SSE event and **ends the turn**; the frontend shows a
confirmation card.

After the user approves or rejects, `POST /ai/agent/approve` restores the full
message sequence from the database and resumes:

- tool results are always persisted in call order and the run stops at the first
  unconfirmed call, so **the index of the suspended call equals the number of
  tool messages following that assistant message** — resumption needs no extra
  bookkeeping.
- Calls queued behind it in the same turn get an explicit "not executed" result,
  keeping the transcript valid for the model.

## 6. Tool inventory (29)

| Class | Count | Examples |
|-------|-------|----------|
| L0 read-only | 16 | `sys_overview`, `list_processes`, `list_ports`, `dir_usage`, `read_file`, `search_logs`, `query_metrics`, `docker_ps`, `security_status` |
| L1 low-risk write | 5 | `clean_temp`, `clean_logs`, `service_control`, `docker_control`, `install_skill` |
| L2 high-risk write | 8 | `exec_command`, `write_file`, `delete_path`, `modify_sshd`, `compress_dir`, `cron_write`, `kill_process`, `firewall_rule` |
| External MCP | dynamic | `mcp__<server>__<tool>`, risk derived from the `readOnlyHint` annotation |

Tools inherit the panel's RBAC: a read-only role never sees tools that need write
permissions, and L2 tools additionally require the `ai.agent.write` permission.

## 7. MCP in both directions

### As an MCP client (consume external tools)

Configure MCP servers under "AI Assistant → MCP" (stdio command or Streamable
HTTP URL with custom headers). The kernel connects with the official MCP Go SDK,
lists the tools and registers each one as `mcp__<server>__<tool>`. Connections are
reconciled every two minutes, so configuration changes need no restart.

### As an MCP server (publish capabilities)

The panel publishes its own tool set at `/mcp` (Streamable HTTP):

- Authentication uses a **bearer token** generated on first start and viewable /
  copyable / rotatable under "AI Assistant → Tool permissions". MCP clients
  cannot perform the panel's JWT login, hence a dedicated token.
- Only **L0 / L1 tools** are published by default. L2 tools stay hidden until
  "Allow high-risk MCP operations" is enabled, because an external MCP client
  cannot present the panel's confirmation dialog.
- The L3 rails and the panel self-protection apply to remote calls too.
- Every remote invocation is written to the audit log with the actor `mcp-client`.

Client configuration example (Claude Desktop / Cursor and other Streamable HTTP
clients):

```json
{
  "mcpServers": {
    "opsmini": {
      "type": "http",
      "url": "https://your-panel.example.com/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

## 8. Sessions and memory

- `ai_sessions` / `ai_messages` live in SQLite and record `tool_calls` and
  `tool_call_id`, so a page reload or a process restart can resume or replay the
  thinking chain.
- When the transcript grows past a threshold (24 messages kept verbatim, plus a
  12-message buffer) it is **compressed**: older turns are summarised by the model
  into a ≤300-character digest and re-inserted as one system message.
- Sessions are retained for 30 days by default (`ai_session_retention_days`) and
  cleaned up in the background.

## 9. API surface

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/v1/ai/agent/chat` | Streaming chat (thinking chain / tool calls / confirmations) |
| POST | `/api/v1/ai/agent/approve` | Approve or reject a suspended call and resume |
| GET | `/api/v1/ai/agent/tools` | Tool inventory (risk, permission, policy) and MCP status |
| PUT | `/api/v1/ai/agent/tools/policy` | Per-tool policy (auto / confirm / disabled) |
| GET | `/api/v1/ai/agent/sessions` | Session list |
| GET | `/api/v1/ai/agent/sessions/:id/messages` | Transcript (including pending approvals) |
| DELETE | `/api/v1/ai/agent/sessions/:id` | Delete a session |
| GET | `/api/v1/ai/agent/config` | Runtime configuration and model health |
| GET | `/api/v1/ai/agent/sessions/:id/report` | Download a session's Markdown report |
| GET | `/api/v1/ai/agent/reports` | List archived report files (with format + archive dir) |
| GET | `/api/v1/ai/agent/reports/view?name=` | **Open** an archived report inline (also accepts `?token=`) |
| GET | `/api/v1/ai/agent/reports/download?name=` | Download an archived report (also accepts `?token=`) |
| GET/PUT | `/api/v1/ai/agent/mcp-server` | Inbound MCP endpoint config (enabled / token / high-risk) |
| ANY | `/mcp` | Inbound MCP endpoint (bearer token) |

Permissions: `ai.agent` (use the agent) and `ai.agent.write` (authorise
high-risk actions). The older `/ai/chat` and `/ai/chat/stream` endpoints remain as
a fallback channel.

## 10. Who owns multi-host: the panel stays single-host

OpsMini is a **single-host panel**; its AI tool set operates on this machine only. Multi-host work belongs to OpsAnt — that is the hub, and it owns three execution engines (salt / ansible / opsmini) plus a unified execution API (`/api/opsant/v1/tasks/run-shell/`, `tasks/run-script/`, `tasks/send-file/`, `tasks/get-minion-file/`, ...).

Earlier versions shipped four multi-host tools (`remote_list_hosts`, `remote_exec`, `remote_pull_file`, `remote_push_file`) that dispatched tasks through the OpsMini Server control-plane HTTP API on port 4508. They were removed because they crossed an architectural boundary:

- 4508 is an **internal** API the OpsMini Server offers to OpsAnt (Django). It is scoped to "local host / internal network" and performs no authentication;
- on that path the panel is the **managed Agent**. It should talk to the Server only over the gRPC connection on 4507 (register, heartbeat, receive tasks) and must not call the control plane back;
- making that flawed design work would require exposing 4508 — i.e. putting an unauthenticated remote command entry point on the internet.

The split is therefore: **single-host operations and single-host AI live in OpsMini; multi-host orchestration and multi-host AI live in OpsAnt.** They connect through "OpsMini as one enrolment method of OpsAnt", and the data flow is always OpsAnt → Agent.

## 11. Scheduled AI tasks (host inspection and friends)

An "AI task" is not a new entry point: it is the **fourth kind of scheduled task**
(`kind = agent`), sitting next to "panel tasks / system crontab / user crontab".
Scheduling, enable/disable, last-run bookkeeping and failure alerts are therefore
all reused, and "run something every morning" lives where users already look.

### Pipeline

```
cron fires -> agent runs unattended (ReAct with read-only / low-risk tools)
           -> transcript stored (the report IS an AI session, thinking chain included)
           -> HTML + Markdown archived under <data_dir>/reports/<task name>/
           -> summary pushed to the task's notification channels
```

### Confirmation policy degrades when unattended

Nobody is around to click "Approve", so with `RunOptions.Unattended = true`:

- a tool mapping to `PolicyConfirm` (L2) is **refused outright**; the model is told
  why ("use a read-only tool instead and state in your conclusion that this step
  needs a human") and the refusal is recorded in the session's `Skipped` list;
- the model switches to read-only tools and the task still finishes, with the
  report naming the operation that needs a human and the exact command to run;
- L0 read-only and L1 low-risk tools are unaffected. A daily inspection is thus
  read-only by construction; widening it requires an explicit policy change.

Refusals land in `cron_jobs.skipped_tools` and show up in both the notification and
the report appendix.

### Reliability and cost control

| Mechanism | Notes |
|-----------|-------|
| Step budget | per-task `max_steps` (default 12, editable in the form) |
| Wall-clock cap | per-task `timeout_seconds` (default 600s); a timeout counts as a failure |
| Auto-disable | 3 consecutive failures disable the task, so a broken job cannot alert forever |
| Manual reset | saving the task in the panel clears `fail_count` |

### Reports: a single HTML file

Each run produces **one self-contained HTML file** — the artifact you open or download:

```
<data dir>/reports/<task name>/<task name>-<timestamp>.html   <- the report (self-contained)
<data dir>/reports/<task name>/<task name>-<timestamp>.md     <- same content as Markdown
```

`<data dir>` is the panel's data directory (`appstore_data_dir`, default
`/var/lib/opsmini/data`). **Every generated artifact lives under it, one subdirectory
per feature** — reports in `reports/`; add sibling directories for future generators.

"Self-contained" means: inline styles, no script, no external stylesheet, no web font,
no images. A report therefore renders from a USB stick, as a mail attachment, or from
any static host. The panel deliberately bundles **no CJK font** (it would add 10 MB to
the single binary), so reports use the reader's own system fonts, which every desktop
and mobile browser has.

Contents: an execution summary (status / steps / tool calls), the task instruction, the
conclusion, and an **Appendix: execution log** with one row per tool call — arguments and
observation side by side, status being one of "done / refused / hard-blocked / failed".

Each task row offers two entry points:

- **Open** — `GET /ai/agent/reports/view`, `Content-Disposition: inline`, rendered in a
  new browser tab;
- **Download** — `GET /ai/agent/reports/download`, `Content-Disposition: attachment`.

The body is model-written Markdown mixed with host data (log excerpts, process command
lines, config values). The renderer escapes the whole document **before** applying any
inline rule and refuses `javascript:`-style targets, so a `<script>` in a log line is
displayed as text. A strict CSP is sent alongside as a second line of defence.

For PDF, print the open report from the browser ("Print → Save as PDF"); the stylesheet
carries print rules so the output matches the screen.

### Exporting ad-hoc conversations

Any AI assistant conversation can be exported as the same HTML report:
`GET /ai/agent/sessions/:id/report` (HTML by default, `?format=md` for Markdown,
`?download=1` to force a download), so sessions that were never archived still yield a
readable report.

### Shared notification path

AI tasks and alerts share one `Notifier` (`internal/service/notify.go`) covering the
in-panel bell, email and WeCom / DingTalk / Feishu group robots. An empty channel list
defaults to the in-panel bell.
