# Autonomous Agent (Event Center)

The **AI Assistant** is *you ask, it answers*: you describe a symptom, it investigates and reports.
The **autonomous agent** is *it watches for you*: collectors observe the host continuously, open an investigation when something deserves attention, decide "record only / ask first / act automatically" within the boundaries you set, and leave every judgement, piece of evidence and action on record.

Both share the same model, toolset and guardrails. The only difference is **who starts it**.

## The loop

```
Collectors (resident, 2s–20s by class)
   ↓  normalised into Signals (fingerprint dedupe, persisted)
Aggregation (the same thing folds into one Incident — counters grow, rows don't)
   ↓
Event Center (the list you see)
   ↓  matched against policies by family + severity
Policy engine (shadow = analyse only / assist = read auto, write needs approval / auto = whitelisted low-risk writes)
   ↓
Multi-step agent investigation (ReAct, every tool call recorded)
   ↓  actions bounded by write budget + cooldown + risk level
Verification (optional assertions: did the action actually take effect?)
   ↓  a failed verification earns one retry
Record (incident → run → each action → report → shadow review)
```

## Collectors

Six resident collectors, all read-only:

| Collector | What it observes | Typical severity |
|---|---|---|
| Log growth | Hourly-bucket EWMA baseline + 20MB/min hard threshold; finds the process holding the file | high |
| Login anomaly | Off-hours, first-seen IP, success after brute force, `authorized_keys` just modified | high |
| Script dropped in web root | 14 WebShell signatures, strong matches go straight to high | high |
| Suspicious process | Web user spawning shell / curl / python, miner names | critical |
| Outbound anomaly | Connections to unexpected hosts and ports | high |
| Metric alerts | 24 rules over CPU / memory / disk / load / service state | medium–high |

Intervals are graded by class (2s–20s). Everything is polling or file reads — no resident agent, no system configuration changes.

## Aggregation: one row or many

The same thing failing repeatedly must be one incident with a counter, not a wall of rows:

| Case | Behaviour |
|---|---|
| Same object changing repeatedly (config edits, restart loop) | Folds while unhandled, `count` grows |
| One scenario from many sources (10 IPs brute forcing) | Folds into one; members expandable (capped at 50, true total kept) |
| **Metric alerts / log growth** | **Fold for as long as the incident is unhandled** — an alert nobody dealt with is one to-do, not a new incident every 10 minutes |
| Login anomaly (successful login) | Same IP folds within half a day; a new day is a new observation |
| Recurrence after ignore / resolve | **Opens a new incident**, never merges back |

> Before v1.4 metric alerts were windowed at 10 minutes, so one CPU rule could produce eight pending rows overnight. Now folding continues while the incident is open: one alert, one row.

## Policies

Policies are **data** (editable in the panel), not code. Each decides: which scenario, from which severity, in which mode, with which budget.

| Mode | Reads | Writes | Use for |
|---|---|---|---|
| `shadow` | automatic | **always refused**, records intent only | observe a new scenario for a couple of weeks |
| `assist` | automatic | needs human approval | the recommended default |
| `auto` | automatic | **only whitelisted low-risk actions** run automatically | scenarios you have validated |

Six built-in policies:

| Policy | Scenario | Default mode | What it may do |
|---|---|---|---|
| `webshell-respond` | WebShell | assist | gather evidence → quarantine file, freeze process, temporarily block source IP (all reversible) |
| `auth-anomaly` | Login anomaly | assist | gather evidence → temporarily block the source IP |
| `proc-anomaly` | Suspicious process | assist | gather evidence → freeze (never kill) |
| `log-growth` | Log growth | shadow | find who writes and why; record intent only |
| `net-anomaly` | Outbound anomaly | shadow (disabled by default) | read-only triage |
| `change-review` | Suspicious change | shadow (forced) | judge whether the edit is malicious; recommend rollback or allow |

Each policy also carries four budgets and one cooldown: **max writes, max steps, max seconds, max tokens, cooldown seconds**. Exceeding any of them ends the run and records it honestly.

When a cooldown blocks a run, the incident says so explicitly — "matched policy X but it is cooling down (N remaining), no automatic analysis this time, please decide manually" — instead of silently doing nothing.

## Attack chains

Single incidents are easy to miss: a webshell at 02:10, a suspicious process at 02:14, an outbound connection at 02:20 read like three unrelated events. Attack chains group them into one intrusion.

Five stages: **initial access → execution → persistence → c2 → impact**.

Evidence used (all explainable, all arguable):

- Shared indicator (same IP, same path root, same domain)
- Temporal proximity (within 15 minutes is strong, within an hour is weak)
- Stage progression (a new stage adds weight; going backwards needs stronger shared evidence)
- A combined score of at least 0.45 is required to attach

Deliberately tight rules:

- **A brute-force burst alone never opens a chain.** Every internet-facing host is scanned constantly; treating failed logins as "the start of an attack" would destroy the feature's credibility. It only joins once something executes or calls out
- **Metric alerts and log growth never join a chain** — symptoms, not stages
- **Changes do not join by default**, unless all three hold: the rule pass judged it suspicious, the target survives a reboot (`authorized_keys`, cron, systemd, sudoers, `ld.so.preload`, PAM, profile), and the chain already saw execution or c2
- **Three stages, or "execution + c2" together**, is what escalates to critical and notifies

> A chain only raises priority. **It never widens the agent's permissions** — budget, mode and available tools are unchanged, and being part of a chain never permits an extra write.

## Handling as a transaction

Every write the agent performs is journaled:

- **Reversible actions** (quarantine, freeze, block, truncate log, snapshot) are rolled back **in reverse order** when handling fails
- **Irreversible actions** (delete, execute command, edit config) cannot be rolled back, and the report says so plainly — "N items cannot be undone, manual review needed" — instead of pretending
- Rollback results are split into three honest buckets: undone / not undoable / rollback failed

The incident detail page lets you roll back any un-rolled action yourself, without waiting for the agent.

## Shadow review

A `shadow` run gets an automatic retrospective: judging from **facts**, was the intended action still needed?

| Verdict | Meaning |
|---|---|
| `would_help` | It would have helped (the problem persisted) |
| `unnecessary` | It was not needed (problem resolved itself / false positive) |
| `unknown` | Not enough evidence to conclude |

This is what turns shadow mode from "watch only" into "practice with a grader": a run of `unnecessary` means thresholds are too loose; a run of `would_help` means it is time to move to assist.

## Feedback suppression

When the same incident group has been ignored twice within 7 days, later occurrences **no longer wake the agent** — they are still recorded and still listed, just not analysed automatically.

The reasoning is simple: one ignore can be a one-off, two is a configuration problem. This is what stops alerts you don't care about from burning tokens.

## Change events are read-only, always

Whatever a policy says, incidents with `family=change` **may investigate but never act**:

- forced to shadow, write budget zeroed, all write tools denied, step cap 8

This is not fussiness, it is attack surface: if writing a monitored config file could make the agent perform an action, that would be a remote trigger vector.

## Using the panel

The **Event Center** page has six filters, each with a count:

| Filter | What it shows |
|---|---|
| Pending | Happened, nobody dealt with it yet |
| Awaiting | Not yet at trigger conditions, waiting for more signals |
| All | No filter |
| Handled | Investigation finished or manually resolved |
| Failed | Run failed / verification failed |
| Ignored | You decided it does not matter |

The detail view drills into: related signals, agent runs, the action chain (tool, arguments, verdict, raw output), report, attack-chain context.

Two manual actions:

- **Ignore** — mark as not worth handling; feeds the suppression counter
- **I'll take over** — "I'm handling this myself"; the agent stops acting (new signals are still recorded)

## Scheduling and audit trail

- Every run leaves an `AgentRun` row (mode, steps, tools, writes, duration, conclusion, retry)
- Reports are archived as single-file HTML, openable or downloadable per incident
- Every action (tool call, approval, refusal, rollback) goes to the audit log

## Related

| Module | Relationship |
|---|---|
| [Change awareness](change-awareness.md) | Supplies "who changed what, when" — the source of suspicious-change incidents |
| [AI assistant](ai-assistant.md) | Same tools and guardrails, started by you instead |
| [Host security](security.md) | Baseline and threat findings enter the event center as signals |
| [Backup & restore](backup-restore.md) | Backup failures create incidents; restore failures raise critical ones |
| [Log pipeline](log-pipeline.md) | Collected logs can serve as evidence |

## The boundary, in one line

OpsMini does **not** ship an unattended root agent. The intended shape is "autonomous investigation + graded authorisation": the agent finds, gathers evidence, judges and recommends; destructive actions need a human or a whitelist.
