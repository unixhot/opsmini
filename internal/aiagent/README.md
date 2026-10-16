# internal/aiagent

The OpsMini AI Agent: a self-built, dependency-light ReAct
(Reason → Act → Observe) kernel with tool calling, risk-graded guard rails and
human-in-the-loop confirmation.

`internal/agent` is a different thing — that package is the gRPC client that
connects the panel to an OpsAnt "OpsMini Server". This package is the LLM agent.

## Why a self-built kernel

The agent frameworks available for Go (Google ADK, CloudWeGo Eino) all require
their own tool abstraction. OpsMini's value lives in its domain tool set, its
safety rails and its gRPC channel, none of which a framework provides. Tools are
therefore declared with plain **JSON Schema (MCP compatible)** instead of a
framework-specific type, which keeps the agent portable and lets the very same
registry be published as an MCP server.

## Files

| File | Purpose |
|------|---------|
| `types.go` | `Tool`, `Result`, `Message`, `ToolCall`, `RiskLevel` — the core data model |
| `registry.go` | Thread-safe tool registry; converts tools to OpenAI and MCP definitions |
| `llm.go` | OpenAI-compatible client with **streaming tool-call fragment accumulation** |
| `loop.go` | The ReAct kernel: reason → act → observe, with suspend/resume |
| `guard.go` | L3 hard-forbidden command patterns, panel self-protection, risk→policy mapping |
| `hitl.go` | Human-in-the-loop: persisted approvals with expiry |
| `memory.go` | Conversation persistence and context compression |
| `mcpclient.go` | Outbound MCP client (official Go SDK, stdio + Streamable HTTP) |
| `mcpserver.go` | Inbound MCP server: publishes the tool set to external MCP clients |
| `report.go` | Markdown report builder + archived-report files on disk |
| `events.go` | SSE event model and the `Emitter` interface |
| `tools/` | The built-in tool set (see `tools/README.md`) |

## Risk model

| Level | Name | Behaviour |
|-------|------|-----------|
| L0 | `RiskReadonly` | Runs unattended |
| L1 | `RiskLowWrite` | Asks for confirmation by default; an administrator may opt a tool into unattended execution |
| L2 | `RiskHighWrite` | Always requires a human approval; can never be set to auto |
| L3 | *(pattern based)* | Hard-blocked before execution — `rm -rf /`, `mkfs`, `dd of=/dev/sd*`, fork bombs, flushing the firewall, … Even an explicit approval cannot enable them |

Two extra rails protect the panel itself: operations that would delete or
overwrite the panel data directory are refused, and firewall rules that would cut
off the panel port are rejected.

## Streaming tool calls

OpenAI-compatible providers stream tool calls as fragments: the first frame
carries the call id and function name, later frames append pieces of the JSON
argument string. `LLMClient.ChatStream` stitches the fragments back together by
their `index`. Providers that do not support streaming tool calls at all (no
content and no tool call frames) are detected and retried with a non-streaming
request.

## Human-in-the-loop

When a tool needs approval the loop persists an `ai_pending_approvals` record
and returns; the SSE stream carries a `confirm` event. `POST /ai/agent/approve`
resumes the run from the stored transcript. Approvals expire after 10 minutes and
are swept by a background goroutine.

Because results are appended in call order and the run stops at the first
unconfirmed call, the index of the suspended call can be derived from the number
of tool messages that follow the assistant message — no extra bookkeeping is
needed to resume deterministically.

## SSE event model

| Event | Meaning |
|-------|---------|
| `session` | The session id (created on demand) |
| `step` | A new reasoning step started |
| `thinking` | Incremental assistant text (rendered as the thinking chain) |
| `tool_call` | The model asked to run a tool |
| `tool_result` | A tool finished (ok / summary / detail / duration) |
| `confirm` | A tool is suspended waiting for approval |
| `final` | The final answer |
| `error` | The run failed |
| `done` | The stream is complete |

## Configuration

Everything is read from the panel settings at call time, so changes take effect
without a restart:

| Setting | Purpose |
|---------|---------|
| `ai_base_url`, `ai_model`, `ai_api_key` | Model endpoint (`OPSMINI_AI_KEY` wins for the key) |
| `ai_tool_policy` | JSON map of per-tool policy overrides (`auto` / `confirm` / `deny`) |
| `ai_session_retention_days` | Conversation retention (default 30 days) |
| `ai_remote_server_url`, `ai_remote_server_token` | OpsMini Server control plane for the multi-host tools |
| `mcp_server_enabled` | Whether the inbound MCP endpoint is served (default on) |
| `mcp_server_token` | Bearer token protecting the inbound MCP endpoint |
| `mcp_server_allow_high_risk` | Whether L2 tools are published over MCP (default off) |

## Unattended runs (scheduled AI tasks)

`RunOptions.Unattended = true` marks a run triggered by the scheduler, where nobody
can answer a confirmation dialog. In that mode any tool mapping to `PolicyConfirm`
is **refused instead of suspended**: the refusal reason is fed back to the model
("use a read-only tool and say in your conclusion that this step needs a human"),
recorded in `SendResult.Skipped`, and surfaced in the report appendix. L0 and L1
tools are unaffected, so a daily inspection is read-only by construction.

`Store.BuildReport` renders the transcript into Markdown (summary → instruction →
conclusion → per-call appendix) and `SaveReport` archives it under
`<data_dir>/reports/<task name>/`; `ListReports` / `ReadReport` back the download
endpoints. Sessions created this way use `SystemUserID` (0) so every logged-in user
can read them.
