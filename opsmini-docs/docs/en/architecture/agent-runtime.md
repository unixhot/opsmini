# Autonomous agent runtime

Since v1.3.0 OpsMini is not only a panel: the panel process runs a closed loop of **perceive → decide → act → verify**. This page explains how it works, where the boundaries are, and how to debug it.

> Full design: `docs/v1.3.0-autonomous-agent.md` in the repository. Implementation details: `docs/architecture-agent.md`.

## Five planes

| Plane | Code | Responsibility |
|---|---|---|
| Perception | `internal/sentinel/` | Turn host observations into normalized signals |
| Normalizing | `sentinel.Bus` / `Aggregator` | De-duplicate, persist, fold into incidents |
| Decision | `internal/trigger/` | Decide whether to wake the agent, in which mode and budget |
| Action | `internal/aiagent/` | ReAct loop: gather evidence, judge, act, report |
| Safety | `internal/safety/` | Argument policy, quarantine, snapshots, budget, kill switch, assertions |

```
logs/auth/files/processes/network/metrics → Signal → Incident → Policy → Agent run → Action + Verify
```

## Six collectors

| Collector | Interval | Detects |
|---|---|---|
| logwatch | 10s | Log growth: per-hour-bucket EWMA baseline plus a 20 MB/min hard threshold; resolves the process holding the file; template fingerprinting spots a "log loop" |
| authwatch | 10s | Login anomalies: off-hours, never-seen source IP, success after a brute force, `authorized_keys` just changed |
| filewatch | 20s | New or modified scripts in web roots (14 webshell markers; strong markers are high severity) |
| procwatch | 2s | Miners, reverse shells, **web users spawning shells/curl/python** |
| netwatch | 20s | Egress to suspicious ports, unusual outbound from web users |
| metricwatch | with alerts | Reuses the existing alert rules: a firing alert becomes a signal |

Cross-checking is the point: a webshell is confirmed by "file landed + content markers + process chain" reinforcing each other.

## Three modes

| Mode | Read-only | Low-risk writes | High-risk writes | Use |
|---|---|---|---|---|
| shadow | yes | recorded, not executed | no | first weeks of a new scenario |
| assist | yes | refused, turned into advice | refused | default in production |
| auto | yes | whitelisted tools run | refused | enabled per scenario, deliberately |

In event-triggered runs the **generic command execution tool is not offered at all** in the webshell scenario: it is listed in `deny_tools`, so the model never sees it.

## Irreversible actions become reversible

| Intuitive action | What OpsMini does instead |
|---|---|
| Delete a suspicious file | Move it to quarantine (sha256, mode, owner recorded), one-click restore |
| `kill -9` a suspicious process | `SIGSTOP` first (recoverable with `SIGCONT`); `SIGKILL` needs a human |
| Edit a config file | Snapshot first, rollback available |
| Block an IP | Temporary ban with a TTL that releases itself |
| Truncate a log | Snapshot first, keep the last N lines (the file is not deleted, holders keep their fd) |

## Seven gates

1. **Tool × argument constraints**: beyond L0–L3 there is an argument policy (protected paths such as `/etc/ssh`, protected units such as `sshd`, hard-denied commands).
2. **Blast radius pre-check** before touching a service.
3. **Snapshot and quarantine**: get a way back before acting.
4. **Budget and breaker**: ≤4 writes, ≤12 steps, ≤300s per incident; two failed verifications stop the incident.
5. **Shadow rollout**: new policies start in shadow; "what it would do" is compared with what humans actually do.
6. **Takeover and kill switch**: one click degrades the agent to read-only.
7. **Decision-chain audit**: every tool call's arguments, verdict, result and reversibility are persisted and replayable.

## Event Center

The sidebar page has five tabs:

- **Incidents**: filter by category, level, status and keyword; 20 per page; **Details** shows the related
  signals, the agent runs and the full action chain
- **Policies**: create, edit and delete them; an "How to read these policies" panel explains the three
  modes and every field in plain language
- **Action log**: tool, risk, verdict (allow/deny/shadow/confirm), duration and result, paged and filterable
- **Quarantine & bans**: restore a quarantined file, release an IP ban; long paths and reasons wrap
  inside the cell instead of forcing a horizontal scrollbar
- **Signals**: the raw stream, so you can answer "why didn't it wake me that night", also paged

All four stat cards drill down: **Needs attention** jumps to the incident list filtered to everything a
human still has to look at (open + triggered + running + failed), **Actions 24h** jumps to the action log.

### What wakes the agent

Three conditions must all hold: matching category, severity at or above `min_severity`, and outside the
`cooldown_sec` window. Otherwise the incident is recorded and no model tokens are spent — this is the main
cost control.

`auto` mode only skips confirmation for the low-risk tools listed in `auto_tools`; it never bypasses the
L0–L3 grading. Anything at L2 or above still needs a human.

### Data retention

Collection is write-heavy, so records expire automatically — once an hour:

| Table | Default | Removed when |
|---|---|---|
| signals | 14 days | expired **and** its incident is closed |
| incidents | 90 days | expired and closed (resolved/ignored/taken_over/failed) |
| action log | 180 days | expired (audit kept longest) |
| agent runs | 90 days | expired and its incident is closed |
| quarantine | 30 days | only restored rows; the file itself is never touched |
| IP bans | 30 days | only released rows |

Three hard rules: **open incidents are never deleted** (with their signals and action chain),
**quarantined files stay until restored**, and **bans stay until released**. Set the days in the panel;
0 means keep forever. Changes apply immediately and survive a restart. The manual cleanup has a
preview button so you can see the count before deleting anything.

## Troubleshooting

| Symptom | Look at |
|---|---|
| Event Center stays empty | `collectors` and `signals_24h` from `GET /api/v1/sentinel/stats`; is the learning period on |
| Incidents but no agent run | policy `min_severity`/`mode`/`enabled`; cooldown; is a model configured |
| Agent ran but did nothing | action log verdicts: `shadow` (shadow mode) or `deny` (needs a human) |
| Action was refused | `rule` in the action log: `protected-path:*`, `kill-switch`, `write-budget` |
| Verification failed | the "验证：" line in `agent_runs.summary` |

Log keywords: `sentinel:` (perception, bus), `trigger:` (policies, scheduling).

## Known limitations

1. No eBPF: millisecond processes and in-memory implants are invisible (optional plugin in v1.4).
2. Only file-based auth logs are parsed; journald-only distributions need to write logs to disk.
3. Baselines live in memory: logwatch re-learns after a restart (hard thresholds are unaffected).
4. Single-host serialization: concurrent incidents queue up, they are never run in parallel against the same machine.
5. Assist does not resume: write actions become advice a human executes in the AI assistant.
