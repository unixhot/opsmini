# AI Assistant

OpsMini has built-in AI large-model operations capabilities, using natural language to perform system diagnostics, log analysis, and command execution.

## Capabilities

- **Natural-language diagnostics**: describe the fault symptoms, and the AI gives diagnostic suggestions based on real-time system status
- **Log analysis**: let the AI analyze abnormal logs and locate the root cause of errors
- **Command execution**: the AI converts natural-language intent into operations commands, which are executed after permission confirmation
- **Deep thinking**: optional "think first, then answer" mode for complex reasoning problems
- **Built-in tools**: injects host system information in real time (CPU/memory/disk/processes/ports/firewall), and supports searching for and installing skills in SkillHub via function calls

## Supported LLM Providers

| Provider | Description |
|----------|------|
| `openai` | OpenAI-compatible interface |
| `deepseek` | DeepSeek |
| `qwen` | Qwen (DashScope-compatible interface) |
| `ollama` | Local Ollama deployment |

## Configuration

Configure everything in **Panel "Settings → AI Model Integration"** (the original `ai` section of `config.yaml` has been migrated here and takes effect immediately at runtime):

- Enable AI assistant (master switch)
- Model provider / model name / API Key / API address
- Natural-language execution / intelligent log analysis / intelligent alert diagnosis toggles

> Configuration priority: environment variable `OPSMINI_AI_KEY` > panel settings > residual `ai.*` in the configuration file. It is recommended to store the key in an environment variable to avoid writing it to disk. See [Panel Settings](../configuration/panel-settings.md#ai-config) for details.

## Security Mechanisms

- When the AI determines "execution intent", it generates a structured action + parameters
- Execution happens only if it hits the permission allowlist; unauthorized / high-risk operations require a second user confirmation
- Executed actions are recorded in the audit log
- After turning off the "Enable AI assistant" switch, AI-related interfaces are rejected outright

## MCP Management

Supports configuring MCP (Model Context Protocol) services to extend the AI assistant's capabilities and data sources. You can create, read, update, and delete MCP configurations in "AI Assistant → MCP", supporting both command (stdio) and URL (SSE/HTTP) transport methods.

## Skills (Skill Hub)

A built-in "Skills" tab connects to the SkillHub skill marketplace:

- Search and one-click install skills
- Upload local skills and manually create skills
- Enable/disable and delete installed skills
