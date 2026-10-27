# AI Assistant

OpsMini ships with built-in AI operations capability. The assistant is not just "a chatbot that can read system information" — it is an **agent**: given your question it plans on its own, calls tools for real data, observes the results and keeps going until it reaches a conclusion. Every risky action, however, still needs your approval first.

## Capabilities

- **Autonomous multi-step troubleshooting**: describe the symptom and the AI chains several tool calls together (check metrics → inspect processes → read logs) before answering, instead of replying from memory
- **Visible thinking chain**: every tool call appears as a card you can expand to see the tool name, its arguments and the raw output, so the conclusion is auditable
- **29 built-in operations tools**: system metrics, processes, ports, disks, files, logs, services, containers, security and cron
- **Graded authorisation with high-risk confirmation**: read-only actions run on their own; high-risk actions pop a dialog and wait for you
- **MCP in both directions**: consume external MCP servers as a tool source, and publish OpsMini's own tool set as an MCP server for Claude Desktop / Cursor and friends
- **Scheduled AI tasks**: e.g. an automatic host inspection every morning, producing a single-file HTML report pushed to your notification channels
- **Persistent sessions**: conversations are stored, so a page reload or process restart can replay the thinking chain; long histories are compressed automatically
- **Skills (Skill Hub)**: search, install and manage skill packages from the marketplace

## Tools and risk levels

Everything the AI can do is defined as a **tool**, and every tool carries a risk level:

| Level | Meaning | Default behaviour |
|-------|---------|-------------------|
| **L0 read-only** | Reads information, changes nothing | Runs unattended |
| **L1 low-risk write** | Bounded writes (clean temp files, restart a service, ...) | Confirmed by default; an administrator may opt an individual tool out |
| **L2 high-risk write** | Running commands, writing or deleting files, changing SSH or firewall config, ... | **Needs human approval every time**; can never be set to auto |
| **L3 forbidden** | Formatting disks, `rm -rf /`, flushing firewall rules, ... | **Blocked before execution**, even with approval |

Representative tools per level:

| Level | Examples |
|-------|----------|
| L0 read-only | `sys_overview`, `list_processes`, `list_ports`, `list_disks`, `dir_usage`, `read_file`, `search_logs`, `query_metrics`, `docker_ps`, `security_status` |
| L1 low-risk write | `clean_temp`, `clean_logs`, `service_control`, `docker_control`, `install_skill` |
| L2 high-risk write | `exec_command`, `write_file`, `delete_path`, `modify_sshd`, `compress_dir`, `cron_write`, `kill_process`, `firewall_rule` |

Tools also respect the panel's RBAC: a read-only role never sees tools that need write permissions, and L2 tools additionally require the `ai.agent.write` permission.

The full inventory lives under **"AI Assistant → Tool permissions"**, where you can also change each tool's policy (**auto / confirm / disabled**).

## Human approval

When the AI wants to run a high-risk action it does not simply do it:

1. A **confirmation card** appears in the conversation listing the tool, the full arguments and the command about to run (`exec_command`, for instance, shows the whole command line);
2. Click **Approve** and the action really runs, with the output fed back to the AI to continue reasoning;
3. Click **Reject** and nothing runs — the AI receives the reason, explains the impact and offers an alternative.

Approval requests expire after 10 minutes by default. Both approvals and rejections are written to the audit log.

## Security mechanisms

- **Scan before executing**: every tool call goes through the L3 forbidden-pattern scan first; a hit is refused and the reason is fed back to the model
- **Panel self-protection**: anything that would damage the panel itself (its data directory, its listening port) is refused
- **Unattended downgrade**: a scheduled task has nobody to click Approve, so L2 actions are refused automatically and recorded in the report while the task finishes its remaining steps
- **Full audit trail**: tool calls, approvals and rejections all land in the audit log
- **Master switch**: turning off "Enable AI assistant" makes the AI endpoints refuse outright

> Product judgement: there is no unattended root agent. The target shape is **"autonomous planning + graded authorisation"** — the AI investigates and advises, a human authorises anything destructive.

## Scheduled AI tasks

An "AI task" is not a separate entry point — it is the **fourth kind of scheduled task**, sitting next to "panel tasks / system crontab / user crontab". Scheduling, enable/disable, last-run time and failure alerts are therefore all reused.

Configure it under **"Cron Jobs → New Task → Task type: Agent task"**:

- **Prompt**: what the AI should do. Five presets are built in (host inspection / security inspection / disk-space inspection / service-state inspection / log-anomaly inspection) — one click fills the field
- **Schedule**: a standard cron expression, with common presets (daily at 08:00, ...)
- **Max steps**: caps how many tool calls the AI may make, limiting runtime and token cost
- **Notification channels**: panel / email / WeCom / DingTalk / Feishu

Each row in the task list offers **Run** (execute once now) and **Report**. A task that fails three times in a row is disabled automatically, so a broken job cannot alert forever.

## Reports

Every run produces a **single-file HTML report** — styles inlined, no script, no external dependencies. It opens anywhere and survives being attached to an email or copied to a USB stick.

```
<data dir>/reports/<task name>/<task name>-<timestamp>.html     <- the report (self-contained)
<data dir>/reports/<task name>/<task name>-<timestamp>.md       <- same content as Markdown
```

The data directory is set under **Panel "Settings → Data storage"** (default `/var/lib/opsmini/data`). Every generated artifact lives under it, one subdirectory per feature.

Contents: an execution summary (status / steps / tool calls), the task instruction, the conclusion, and an **Appendix: execution log** with one row per tool call — arguments and observation side by side, status being one of "done / refused / hard-blocked / failed".

Each task row has two entry points:

- **Open** — renders the report straight in a new browser tab, no download dialog
- **Download** — saves it as an `.html` file

For PDF, print the open report from the browser ("Print → Save as PDF"); the stylesheet carries print rules so the output matches the screen.

## Supported LLM providers

| Provider | Description |
|----------|-------------|
| `openai` | OpenAI-compatible interface |
| `deepseek` | DeepSeek |
| `qwen` | Qwen (DashScope-compatible interface) |
| `ollama` | Local Ollama deployment |

Any service speaking the OpenAI `chat/completions` protocol works. Tool calling requires a model with function-calling support; models without it fall back to plain conversation automatically.

## Configuration

Configure everything in **Panel "Settings → AI Model Integration"** (the original `ai` section of `config.yaml` has been migrated here and takes effect immediately at runtime):

- Enable AI assistant (master switch)
- Model provider / model name / API Key / API address
- Natural-language execution / intelligent log analysis / intelligent alert diagnosis toggles

> Configuration priority: environment variable `OPSMINI_AI_KEY` > panel settings > residual `ai.*` in the configuration file. It is recommended to store the key in an environment variable to avoid writing it to disk. See [Panel Settings](../configuration/panel-settings.md#ai-config) for details.

Once configured, the runtime figures at the top of "AI Assistant → Tool permissions" tell you whether the model is ready (`model_ready`).

## MCP management

### As a client: consuming external tools

Create, update and delete MCP servers under "AI Assistant → MCP" to extend the AI's capabilities and data sources. Two transports are supported:

- **Command (stdio)**: e.g. a locally running MCP server process
- **HTTP (Streamable HTTP)**: a remote MCP server, with custom request headers

Once connected, external tools appear in the inventory as `mcp__<server>__<tool>`, with the risk level derived from the read-only hint the server declares. Connections are reconciled every two minutes, so configuration changes need no restart.

### As a server: letting other AIs drive OpsMini

The panel can also publish its own tool set for MCP clients such as Claude Desktop, Cursor or OpsAnt:

- The endpoint is `/mcp` (Streamable HTTP), authenticated with an **access token** you can view, copy and rotate under "AI Assistant → Tool permissions"
- **Only read-only and low-risk tools are published by default.** High-risk tools (run a command, write a file, ...) stay hidden because an external client cannot present the panel's confirmation dialog; enable "Allow high-risk MCP operations" on the same page if you really need them
- The L3 forbidden rules and the panel self-protection apply to remote calls too

## Skills (Skill Hub)

A built-in "Skills" tab connects to the SkillHub skill marketplace:

- Search and one-click install skills
- Upload local skills and manually create skills
- Enable/disable and delete installed skills
