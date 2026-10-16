# internal/aiagent/tools

The built-in tool set of the OpsMini AI Agent. Every tool is declared with a
plain JSON Schema input, so the same definition is exported to the model as an
OpenAI function and to external MCP clients as an MCP tool.

`Build(registry, deps)` is the single entry point; it is called once by the
router. Tools whose dependencies are missing (no Docker daemon, no remote control
plane) are simply not registered, so the agent degrades gracefully.

## Files

| File | Contents |
|------|----------|
| `deps.go` | `Deps` (service wiring) and `Build` |
| `helpers.go` | Argument decoding, JSON Schema builders, result helpers |
| `readonly_sys.go` | System observation tools |
| `readonly_ops.go` | Panel data observation tools |
| `write.go` | Low and high risk write tools |
| `remote.go` | Multi-host tools over the OpsMini Server control plane |

## Tool inventory

### L0 — read-only (run unattended)

| Tool | Purpose |
|------|---------|
| `sys_overview` | Live CPU / memory / disk / load / counters overview |
| `list_processes` | Processes sorted by memory or CPU, with a name filter |
| `list_ports` | Listening ports with the owning process |
| `list_disks` | Partitions and usage |
| `list_network` | Interfaces with traffic counters, plus the routing table |
| `list_services` | systemd units and their state |
| `dir_usage` | Recursive directory size plus the biggest children (`du -sh *`) |
| `read_file` | Bounded text file read (head or tail) |
| `search_logs` | Log search with a keyword or regular expression |
| `query_metrics` | Recent metric samples over a time window |
| `docker_ps` | Containers, images, volumes or networks |
| `list_cron_jobs` | Panel jobs plus system and user crontabs |
| `list_websites` | Registered websites |
| `list_databases` | Registered databases |
| `security_status` | Security score and per-module summaries |
| `search_skills` | SkillHub marketplace search |

### L1 — low-risk write (confirmation configurable)

| Tool | Purpose |
|------|---------|
| `clean_temp` | Remove stale files from `/tmp` and `/var/tmp` (supports `dry_run`) |
| `clean_logs` | Truncate a whitelisted log file |
| `service_control` | systemd start / stop / restart / reload |
| `docker_control` | Container start / stop / restart |
| `install_skill` | Install a skill from SkillHub |

### L2 — high-risk write (always confirmed)

| Tool | Permission | Purpose |
|------|-----------|---------|
| `exec_command` | `ai.agent.write` | Run a shell command |
| `write_file` | `ai.agent.write` | Write a file (backed up first) |
| `delete_path` | `ai.agent.write` | Delete a file or directory |
| `modify_sshd` | `ai.agent.write` | Change `/etc/ssh/sshd_config` (validated, auto-rolled back) |
| `compress_dir` | `ai.agent.write` | Archive a directory |
| `cron_write` | `ai.agent.write` | Create a panel scheduled task |
| `kill_process` | `ai.agent.write` | Signal a process |
| `firewall_rule` | `ai.agent.write` | Open/close a port, enable/disable the firewall |

### Multi-host (registered only when a remote control plane is configured)

| Tool | Risk | Purpose |
|------|------|---------|
| `list_remote_hosts` | L0 | Enrolled hosts and their online state |
| `remote_exec` | L2 | Run a command or script on another host (no SSH) |
| `remote_pull_file` | L1 | Fetch a file from another host |
| `remote_push_file` | L2 | Write content to another host |

### External (registered dynamically)

Tools discovered from MCP servers configured in the panel are namespaced as
`mcp__<server>__<tool>`. Their risk is derived from the server's
`readOnlyHint` annotation: read-only hints become L0, everything else L1.

## Adding a tool

```go
func myTool(d Deps) aiagent.Tool {
    return aiagent.Tool{
        Name:        "my_tool",
        Title:       "My tool",
        Description: "What it does, and when the model should call it.",
        Risk:        aiagent.RiskReadonly,
        Perm:        "system.view", // optional extra permission
        Schema: schema(map[string]any{
            "path": pStr("Absolute path"),
        }, "path"),
        Preview: func(args map[string]any) string { return "Read " + str(args, "path") },
        Execute: func(ctx aiagent.ExecContext, args map[string]any) (aiagent.Result, error) {
            if str(args, "path") == "" {
                return failed("path is required"), nil
            }
            return textResult("done"), nil
        },
    }
}
```

Then add it to the matching slice in `Build`'s helpers (`readonlyTools`,
`lowRiskTools`, `highRiskTools`).

Conventions:

- Prefer `failed(...)` over returning an error: a tool failure is fed back to the
  model as an observation, while a returned error aborts the step.
- `Preview` is shown in the confirmation dialog, so it must describe the concrete
  effect (the exact command, path or port).
- Keep `Summary` compact — it is what the model reads; put long output in `Detail`.
