# OpsMini AI Agent Design

> Milestone M8 ｜ Code: `internal/aiagent/`

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
| Multi-host (opt-in) | 4 | `list_remote_hosts`, `remote_exec`, `remote_pull_file`, `remote_push_file` |
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
| GET/PUT | `/api/v1/ai/agent/mcp-server` | Inbound MCP endpoint config (enabled / token / high-risk) |
| ANY | `/mcp` | Inbound MCP endpoint (bearer token) |

Permissions: `ai.agent` (use the agent) and `ai.agent.write` (authorise
high-risk actions). The older `/ai/chat` and `/ai/chat/stream` endpoints remain as
a fallback channel.

## 10. Multi-host management

Once the panel has an `agent.server_addr` (the OpsMini Server data plane on the
OpsAnt host), four multi-host tools are registered automatically. They drive the
Server's control-plane HTTP API (port 4508 on the same host by default) to run
commands or scripts and move files on other enrolled hosts — no SSH anywhere.
`ai_remote_server_url` / `ai_remote_server_token` override the derived address.
