# Host Security Design

> This document explains **how the "Host Security" feature is designed and implemented**: what each check examines, what the criteria are, how scores are calculated, where the boundaries lie, and why these choices were made.
>
> Corresponding code: `internal/service/{security,baseline,loginsecurity,firewall,fim,threat,securitymonitor}.go`, `internal/api/v1/{security,baseline,fim,threat,firewall}.go`, `internal/router/router.go`.

---

## 1. Design Goals and Non-Goals

### Goals

| Goal | Specific meaning |
| --- | --- |
| **Zero dependencies** | No agent to install, no kernel modules, no connection to a central service; the binary can be audited as soon as it is brought up |
| **Explainable** | Every check can be opened to see "which file was read, what value was seen, why it was judged non-compliant, and how it is suggested to be fixed" |
| **Actionable** | After a problem is found, it can be fixed directly in the panel (change the firewall, run a health check, rebuild the baseline, mark a threat as handled) |
| **For humans and for AI** | The same conclusion is both rendered as a page and handed to the built-in Agent as a read-only tool |
| **No privilege escalation** | Read-only capabilities are available as soon as you log in; any action that modifies the system requires a separate permission point + root identity |

### Non-Goals (these are trade-offs, not unfinished work)

- **Not a HIDS replacement**: no kernel syscall hooks, no eBPF probes, no full-traffic detection. Such capabilities require always-on high-privilege collection, which conflicts with the positioning of "single binary, tens of MB, installable at any time".
- **No multi-host situational awareness**: a single host yields a single-host conclusion. Aggregation across multiple machines is the responsibility of OpsMini Server / Proxy.
- **No built-in rule library engine**: baseline criteria are written in code (14 items); hot-loading of YAML rules is not accepted. Hot-loadable rules sound flexible, but they would make "why does this machine score 62" impossible to reproduce. Externalizing rules is the direction of evolution (see §16).
- **Not a replacement for professional compliance scanning**: no full item coverage or certification for MLPS/CIS; it only covers the part that "can be seen and acted on in daily operations".

---

## 2. Capability Map

The Host Security page has six tabs, essentially **five detection modules + one summary view**:

| Tab | What it detects | Data source | Trigger method | Modifies the system |
| --- | --- | --- | --- | --- |
| Security Overview | Aggregated summary: total score, grade, scores of the five modules, and a one-sentence conclusion | Persisted results of the other five modules | Open the page | No |
| Baseline Check | Current state of 14 host hardening items | `/etc/ssh/sshd_config`, `/etc/passwd`, `/etc/shadow`, `/etc/login.defs`, crontab, listening ports | Manually click "Start Check" | No |
| Login Security | Brute force, abnormal logins, SSH hardening suggestions | `/var/log/auth.log` or `/var/log/secure` | Open the page | No |
| Firewall | Current backend and rules; allow/block ports, start/stop | Actual state of `ufw` / `firewall-cmd` / `iptables` | Manual operation | **Yes** |
| File Integrity | Whether key files have been modified/deleted | SHA256 of 8 critical system files | Background every 60 seconds | No |
| Threat Detection | Mining processes, suspicious cron jobs, suspicious autostart entries | `/proc`, crontab, `/etc/rc.local`, systemd service list | Background every 5 minutes + manual | No (remediation is done by humans) |

One explicit design decision: **only the firewall modifies the system**. The other four modules are pure observers; even a false positive will only make the score look bad, not put the machine at risk.

---

## 3. Architecture: Two Independent Pathways

In OpsMini there are two security-related systems that look similar but are actually two independent pathways:

```
                  ┌──────────── Health Check Pathway (pull, for humans) ────────────┐
  /etc, /proc ───▶ │ BaselineService  Login Analysis  FIM  Threat Scan        │ ──▶ SQLite ──▶ Security Page + Score
  Auth Logs       │ SecurityService Aggregate Scoring                        │
                  └──────────────────────────────────────────────────┘

                  ┌──────────── Sensing Pathway (push, for Agent) ──────────┐
  auth.log, Site   │ sentinel Collector (logwatch/authwatch/           │ ──▶ Signal ──▶ Event ──▶ Policy
  Directories, Process Table     │ filewatch/procwatch/netwatch/metricwatch）      │        ──▶ Agent Remediation
                  └──────────────────────────────────────────────────┘
```

**Why split into two pathways instead of merging into one?**

| Dimension | Health Check Pathway | Sensing Pathway |
| --- | --- | --- |
| Interaction Model | Pull: you open the page / click scan | Push: collectors run continuously |
| Output | Checklist + Score (a state snapshot that can be stored long-term) | Signal → Event (a time-sensitive stream) |
| Consumer | Human | Policy Engine + AI Agent |
| Judgment Character | "Is the current state compliant?" | "What just happened?" |
| Cost of Misjudgment | Score looks bad | May trigger automatic remediation |

Merging would make the two sides drag each other down: health checks need to be recalculable at any time, while sensing needs continuous incremental processing; add a state machine to one pathway, and the other is forced to carry it along.

**The cost must also be stated clearly**: the two pathways each parse authentication logs (`service/loginsecurity.go` and `sentinel/authwatch.go` are two sets of regexes), and there is a risk of drift in their criteria. This is known technical debt, see §16.

---

## 4. Security Scoring Model

The total score is 100, with fixed weights across five modules:

| Module | Max score | Why this weight |
| --- | --- | --- |
| Baseline check | 40 | The only part "you can actively improve," and the best reflection of day-to-day operations maturity |
| Login security | 20 | When a public-facing machine gets compromised, it almost always starts here |
| Firewall | 15 | Present or absent, this directly determines the exposure surface |
| File integrity | 15 | Modified critical files usually mean the host is already compromised |
| Threat detection | 10 | The low weight is because it is the most prone to false positives (see §10) |

### Scoring rules for each module (one-to-one with the code)

**Baseline (0–40)**: weighted by weight, not by item count.

```
got   = Σ(weight of passed items) + Σ(weight of warn items × 0.5)
total = Σ(weight of all items) = 32
score = int(40 × got / total)
```

Three-state semantics: `pass` counts as full score, `warn` counts as half score, `fail` counts as 0. The reason warn exists is that some items are "not done but not wrong" (for example, SSH still using port 22: the risk is real but common, and scoring it 0 outright would make the score lose discrimination).

**Login security (0–20)**:

| Situation | Score |
| --- | --- |
| No brute force detected, no failures in 24h | 20 |
| Failed logins in 24h but below the brute-force threshold | 16 (fixed deduction of 4) |
| n suspected brute-force targets detected | `20 − 5n`, lower bound 0 |

**Firewall (0–15)**: enabled 15, not enabled 0. Binary determination, no intermediate state.

**File integrity (0–15)**: `15 − 3n` (n = number of changes within 24h), lower bound 0. A deduction of 3 points per change means changing 5 critical files brings it to zero.

**Threat detection (0–10)**: starts from 10 and deducts, `critical` deducts 5, `warning` deducts 2, others deduct 1, lower bound 0.

### Grade mapping

| Score | Grade |
| --- | --- |
| ≥ 90 | Excellent |
| ≥ 75 | Good |
| ≥ 60 | Fair |
| < 60 | Dangerous |

### Trade-offs of this model

- **No asset value weighting**: OpsMini does not know what business is running on this machine, and guessing an "importance coefficient" would only turn the score into mysticism.
- **No CVSS**: CVSS rates vulnerability severity, whereas this rates host configuration and current operating state; the two are not the same thing.
- **No negative scores**: deduction for a single item stops at 0, avoiding the score becoming meaningless after one module drags down the whole.
- **Reproducible score**: same input, same code, same score. This is the prerequisite for using it to compare "take another look after changes."

---

## 5. Baseline checkup

### How it runs

One scan generates one `ScanID` (nanosecond timestamp), and each of the 14 results is written as one line to `baseline_results`. The page only shows the **latest batch**; historical batches remain in the database for comparison.

Pure file parsing, with no calls to external tools such as `lynis` / `oscap` — those tools often do not exist on minimized systems, and their output is hard to structure.

### Decision details

- `sshd_config` reads both the main file and `/etc/ssh/sshd_config.d/*.conf` (modern distributions split configuration into drop-in directories; reading only the main file would cause misjudgment).
- "Not explicitly configured" and "explicitly misconfigured" are distinguished: the former gets `warn` (the distribution default is uncertain), the latter gets `fail`.
- `/etc/shadow` and `/etc/passwd` parse the text directly and do not depend on NSS configuration — avoiding decision drift on machines where LDAP/SSSD is involved.
- High-risk ports use a fixed list: `23 Telnet`, `445 SMB`, `135 RPC`, `1433 MSSQL`, `3389 RDP`.

The complete list of 14 items is in [Appendix A]](#附录-a基线-14-项清单).

### Why the weights are hardcoded in the code

Changing the weights changes the score. If the weights were configurable, the same machine could have different scores at two customers, and the "checkup" would lose its credibility. To adjust the weights, go through code review.

---

## 6. Login Security

### Data Sources and Overhead

Read-only tail of the authentication log: `tail -n 5000`, preferring `/var/log/auth.log` (Debian-based) then `/var/log/secure` (RHEL-based); if neither exists, return an empty report rather than an error. The log may be several hundred MB, and reading it in full would blow up memory.

**The statistics window must be stated accurately**: the window in the implementation is "the last 5000 lines of the log," not a strict time window. The field names follow `failed_count_24h` / `success_count_24h`, because on a public-facing machine that is being scanned heavily, 5000 lines often cover less than 24 hours, while on a quiet machine 5000 lines may span several days. This deviation is a known issue (§15); the reason for not doing strict 24-hour filtering is given below.

### Three Types of Criteria

| Criterion | Rule | Output |
| --- | --- | --- |
| Brute force | The same account fails (including `Failed password` and `Invalid user`) **cumulatively ≥ 5 times** within the statistics window | Account name + count + list of involved IPs, sorted by count in descending order |
| Top failure sources | Aggregate failure counts by IP and take the top 10 | IP + count |
| Abnormal login | Among successful logins, `user == root` | Time + user + IP + reason "root account logged in directly via SSH" |

SSH hardening suggestions are not hardcoded text; instead, the current `sshd_config` is read to determine whether each item has been done (`PermitRootLogin`, `PasswordAuthentication`, changing the port, Protocol 2), and the page directly shows "done/not done."

### Why GeoIP and UA Analysis Are Not Done

Offline environments are a hard requirement. A security panel that can be installed on an intranet, without internet access, and without sending logs to external services is more useful than a panel that can draw a pretty world map but requires internet access. Therefore, anomaly determination relies only on facts visible on the local machine (time, account, IP frequency).

### Why Strict 24-Hour Filtering Is Not Done

Timestamps in `syslog` do not include the year (`Aug 14 07:00:01`); filtering by time requires inferring the year and handling year-wrap rollover; around midnight, inconsistent container time zones, and logs truncated by logrotate can all cause inference errors. Being off by one year means treating the entire history as "just now."

Using "the last N lines" instead of a time window has the cost of a floating window length, but the benefit is that **it never misjudges due to time parsing errors**. If precision is truly needed, the proper approach is to have the perception layer perform window queries in the database according to `observed_at` (which already carries time); this is also one of the motivations for the unified fact layer in §16.

### Explanation of the Threshold of 5

Fewer than 5 failures are mostly manual password typos; 5 or more within 24 hours from the same set of IPs is what exhibits the characteristic of "being tried." On real public-facing machines, this threshold will be exceeded by a large number of scans—and that is precisely its purpose: **it is not meant for precise identification, but for telling you that this machine is being scanned.**

---

## 7. Firewall

### Automatic backend detection

No assumptions are made about what is installed on the system; the actual state is read to determine the backend:

| Backend | Determination | Supported write operations |
| --- | --- | --- |
| `ufw` | `ufw status` readable | Allow/block ports, enable, disable, delete rules by number |
| `firewalld` | `systemctl is-active firewalld` | Allow/block ports (`--permanent` + `--reload`), enable, disable |
| `iptables` | Rule chain exists | Allow/block ports (`-I INPUT 1`), delete rules by number; **disabling within the panel is not supported** |

The `iptables` backend deliberately does not provide "disable": clearing the rule chain is equivalent to leaving all machines exposed, which is too destructive, so users are required to perform the operation on the machine themselves.

### Lockout prevention design (this is the most critical item in this module)

**Before enabling the firewall, automatically allow the panel's own port and SSH 22 first**, then `enable`.

The order must not be reversed. Reversing it causes the classic incident: as soon as the firewall is enabled, 8888 and 22 are both blocked at the same time, leaving only the console available. `panelPort` is injected from the configuration at startup, not hardcoded to 8888.

### Parameter validation and timeout

| Defense | Approach |
| --- | --- |
| Permission | All write operations `requireRoot()`; non-root directly returns `ErrNeedRoot` |
| Port | `int` type + 1–65535 range validation; strings are not accepted |
| Protocol | Enum `tcp`/`udp` |
| Injection | Everything goes through `exec.Command(name, args...)`, not via the shell; parameters cannot be concatenated into a second command |
| Hanging | Each firewall command has a 10-second timeout (`context.WithTimeout`); on failure, the original stderr is returned |

### Relationship with Agent blocking

There are two kinds of blocking in the system, deliberately managed separately:

| Source | Location | Lifecycle | Who cleans up |
| --- | --- | --- | --- |
| Page rules (configured by humans) | `ufw`/`firewalld` rule table | Long-term, until a human deletes them | Humans |
| Agent blocking (event-driven) | `iptables -I INPUT -s <ip> -j DROP` + `blocked_ips` table | With TTL, self-heals on expiry | `SweepBlocks()` automatically unblocks |

Reason for separation: Agent blocking must be able to self-heal (false blocks must be able to recover on their own), while human rules must not be inadvertently deleted by the self-healing logic. The two sets are managed independently and are separately visible on the page.

---

## 8. File Integrity (FIM)

### Monitored objects

By default, 8 critical system files; a baseline is created automatically on first startup (when the baseline is empty):

```
/etc/passwd  /etc/shadow  /etc/group   /etc/ssh/sshd_config
/etc/crontab /etc/sudoers /etc/hosts   /etc/resolv.conf
```

Among these, changes to `/etc/passwd`, `/etc/shadow`, and `/etc/sudoers` are recorded as `critical`, and the rest as `warning`.

### Detection loop

Every 60 seconds, `SHA256 + size + mtime` is computed once for each enabled item:

- Cannot be computed (file does not exist) → record a "file deleted" entry, with the `critical` level determined by path
- Hash mismatch → record a "file content changed" entry, with the old and new hashes
- Match → do nothing

### Key design: baseline automatically advances after a change

After a change is detected and written to the database, **the baseline is immediately updated to the new hash**.

This is intentional: if the baseline did not advance, the same change would be reported every 60 seconds—1440 alerts a day—and people would stop looking at them altogether. With advancement, the effect is "one change, one alert," while **the next change will still be reported**—if it really is modified repeatedly, it remains visible.

Trade-off: if two consecutive changes occur within the same 60-second window, only one record is kept. This is acceptable at this precision (finer granularity would require inotify, see §16).

"Rebuild baseline" is a manual action: after you finish modifying `sudoers`, click it to tell the system "the current state is correct."

### Division of labor with the perception-layer filewatch

|  | Security page FIM | Perception-layer filewatch |
| --- | --- | --- |
| What it watches | 8 critical system files | Newly added executable scripts in the website root directory |
| Criterion | Hash comparison | Extension + WebShell content signatures (`eval($_POST`, `base64_decode`, etc.) |
| Purpose | Detect "the system has been tampered with" | Detect "the website has been implanted with a webshell" |

The two do not overlap and do not reference each other.

---

## 9. Threat Detection

Three scanners, one round every 5 minutes, and one run at startup.

| Scanner | What it looks at | Criteria | Level |
| --- | --- | --- | --- |
| Process | `/proc` full process list (name + command line) | Matches mining keywords (`xmrig`, `minerd`, `kdevtmpfsi`, `kinsing`, `stratum`, etc., 11 in total) | critical |
| Process | Process executable file path | Located in `/tmp` or `/dev/shm` (normal services should not start from here) | warning |
| Cron job | System + user crontab | Command contains `curl`, `wget`, `base64 -d`, `/dev/shm` | warning |
| Startup item | `/etc/rc.local` non-comment lines | Contains `curl`, `wget`, `/tmp` | warning |
| Startup item | systemd enabled service names | Contains `miner`, `xmrig`, `kdevtmpfs` | critical |

### Deduplication and Lifecycle

Deduplicate by `kind + name`: if a hit matches an existing record, only update `last_seen` and the details, without adding a new entry. The only statuses are `active` / `resolved`, and **only a human can mark it as resolved**—before the process is killed, the scanner will see it again every 5 minutes, which is exactly what we want (a real threat should not disappear on its own).

### An Honest Assessment of This Detection

The keyword blacklist **can be bypassed by renaming**, which is a public secret. So its positioning is not "catching all mining," but rather:

1. Catching **lazy attacks**—the vast majority of botnets deploy ready-made binaries without even changing the names;
2. Combined with **behavioral characteristics** such as "executables in temporary directories," covering the renamed portion;
3. The rest is supplemented by process behavior and login behavior signals from the perception layer.

We do not claim it can kill; if it can detect, it reports, and handling is left to humans (or to an Agent authorized by a human).

---

## 10. Data Model

| Table | One row represents | Key fields |
| --- | --- | --- |
| `baseline_results` | One result in a scan | `scan_id` (batch), `category`, `item_key`, `status`](pass/fail/warn), `detail`, `suggestion`, `weight` |
| `fim_baselines` | The current baseline of a monitored file | `path`](unique), `sha256`, `size`, `mod_time`, `enabled` |
| `fim_changes` | A file change event | `path`, `old_sha256`, `new_sha256`, `level`](warning/critical), `message`, `created_at` |
| `threat_findings` | A threat entry | `kind`](process/cron/startup), `name`, `detail`, `severity`, `status`, `first_seen`, `last_seen`, `resolved_at` |

Design highlights:

- Baseline results are batched by `scan_id` and **never overwrite history**: comparing "last week and this week" is queryable.
- `fim_baselines` and `fim_changes` are stored in separate tables: one is state, the other is a log; mixing them in one table would let the "current baseline" and the "change history" contaminate each other.
- `threat_findings` carries `first_seen/last_seen`: at a glance you can tell whether this just appeared or has been sitting there for three days unattended.
- There is no separate security database file; everything lands in `opsmini.db` and follows the backup (consistent with the rest of the panel's data).
- Note the table names: the security baseline results are `baseline_results`, while the perception layer's "learning baseline" (EWMA expected value) is `baselines`; the two are unrelated, don't mix them up.

---

## 11. API and Permissions

All endpoints are under `/api/v1` and accessible after JWT authentication; write operations additionally require permission points.

| Method | Path | Capability | Permission Point |
| --- | --- | --- | --- |
| GET | `/security/overview` | Total score, grade, five-module summary | Login required |
| GET | `/security/baseline/latest` | Latest batch check results | Login required |
| POST | `/security/baseline/scan` | Run a check | `security.scan` |
| GET | `/security/login` | Login security analysis | Login required |
| GET | `/security/login/ssh` | SSH hardening recommendations and current status | Login required |
| GET | `/security/firewall` | Backend, start/stop status, rule list | Login required |
| POST | `/security/firewall/port` | Allow port | `security.firewall` |
| DELETE | `/security/firewall/port` | Block port | `security.firewall` |
| POST | `/security/firewall/enable` | Enable firewall | `security.firewall` |
| POST | `/security/firewall/disable` | Disable firewall | `security.firewall` |
| DELETE | `/security/firewall/rule/:ref` | Delete rule | `security.firewall` |
| GET | `/security/fim/baselines` | Baseline list | Login required |
| POST | `/security/fim/rebuild` | Rebuild baseline | `security.fim` |
| DELETE | `/security/fim/baselines/:id` | Remove a monitoring item | `security.fim` |
| GET | `/security/fim/events` | Change history | Login required |
| GET | `/security/threats` | Threat list | Login required |
| POST | `/security/threats/scan` | Scan now | `security.threat` |
| POST | `/security/threats/:id/resolve` | Mark as handled | `security.threat` |

**Read endpoints never have permission points**: seeing the problems on your own machine requires no authorization; changing the system does. Permission points are held by RBAC roles, and ordinary read-only roles can only view.

---

## 12. Security Boundaries of the Panel Itself

The security module is itself an attack surface, so:

| Risk | Handling |
| --- | --- |
| Command injection | All external commands use `exec.Command` + argument arrays, never shell concatenation; ports are `int`, protocols are enums |
| Privilege escalation | Write operations first `requireRoot()`, non-root is rejected outright, no "best effort" |
| Long hangs | External commands have a 10-second timeout |
| Self-lockout | Allow the panel port and 22 before enabling the firewall |
| Privacy | Only read login events from logs, never store passwords or full command text (only fields used for decisions go into the database) |
| Unauthorized calls | Every write endpoint has its own permission point, not relying on the frontend hiding buttons to "control" access |

---

## 13. Performance and Resource Overhead

| Action | Overhead | Description |
| --- | --- | --- |
| Baseline check | 14 small file reads + one port table, millisecond level | Only runs when a person clicks |
| Login analysis | `tail -5000` once, single run < 50ms | Runs when the page is opened, no caching |
| FIM check | SHA256 of 8 files, every 60 seconds | All are KB-level small files, CPU is negligible |
| Threat scan | Traverses `/proc` hundreds of processes + crontab + systemctl once, every 5 minutes | The only "heavy" action, still on the order of hundreds of milliseconds |
| Resident memory | No additional cache counted, all goes through the same SQLite connection | No separate process, no background queue |

No resident sampler, no extra goroutine pool: FIM and threat scan share one loop (`SecurityMonitor`), one ticker handles one thing.

---

## 14. Relationship with Autonomous Agents

There are three interfaces, all of which **provide facts in one direction only**; there is no coupling such as "the security module commanding the Agent":

| Direction | Form | Description |
| --- | --- | --- |
| Security → AI | Read-only tool `security_status` | The Agent can read the "score + five-module summary" to answer "is this machine secure?", and the tool is `RiskReadonly` + `security.view` |
| Security → AI (write) | Tool `firewall_rule` | Changing the firewall is a write operation, goes through L1/L2 tiering, and requires confirmation |
| Perception layer → Agent | Signal → Event → Policy | Handled independently by sentinel, and does not share state with the security module in this section |

**What is currently not done (intentional)**: threat detection results and FIM changes do not enter the event bus. The reason is that these two types of results have a higher false positive rate than the signals of the perception layer (keyword matching and automatic baseline advancement both produce noise), and feeding them directly into automated response policies is too risky. Connect them after there is measured data on the false positive rate; see §16.

---

## 15. Known Limitations

| Limitation | Impact | Notes |
| --- | --- | --- |
| No kernel-level detection | rootkits and hidden processes (`/proc` hooked) are invisible | Determined by positioning; not planned |
| FIM covers only 8 files | No custom paths, directory recursion, or new file monitoring | Currently supplemented by the perception layer's filewatch for website directories |
| Threat detection relies on keywords | Renaming bypasses it | See the honest assessment in §9 |
| Baseline is fixed at 14 items | Cannot cover the full set of classified protection/CIS items | The evolution direction is to externalize rules |
| Scoring does not include business importance | Test machines and core database servers score the same | Not doing it, to avoid arbitrariness |
| Dependency on authentication log format | Machines whose text format is not rsyslog/journald may not be readable | Compatibility is handled by `/var/log/secure` as a fallback |
| Login statistics window floats | `tail -5000` rows, not strictly 24 hours (the field name still uses 24h) | See §6 "Why strict 24-hour filtering was not implemented" |
| Two sets of log parsing | The security page and the perception layer may drift apart | Known technical debt |
| No report export | Can only be viewed on the page; cannot produce PDF/HTML compliance reports | Evolution direction |

---

## 16. Evolution Direction

In order of priority:

1. **Configurable FIM**: support custom paths, directory recursion, and whitelists (exclude high-frequency changing files under `/etc/` first), and replace 60-second polling with inotify event-driven detection.
2. **Externalize baseline rules**: 14 items → YAML rule packages, support importing CIS / Classified Protection Level 2 item sets, and record "which rule set is currently in use" in the results to ensure scores are reproducible.
3. **Unified log fact layer**: `service/loginsecurity.go` and `sentinel/authwatch.go` share one parser to eliminate definition drift.
4. **Threat detection enhancement**: integrate `ausearch` (auditd audit logs) and optional YARA rules, upgrading from keywords to dual-track behavior and signature detection.
5. **Security results into the event bus**: after the false positive rate becomes quantifiable, connect `critical`-level threats and `critical`-level FIM changes to the policy engine, enabling the Agent to handle them autonomously (e.g., detect WebShell landing → isolate → report).
6. **Compliance report export**: one-click single-file HTML report (reusing the rendering from cron job reports), including scores, per-item conclusions, and remediation recommendations.

---

## Appendix A: Baseline 14-Item Checklist

| Category | Item | Weight | Determination |
| --- | --- | --- | --- |
| ssh | SSH prohibits root password login | 3 | `PermitRootLogin` being `no`/`prohibit-password` is judged pass; not configured is judged warn; otherwise fail |
| ssh | SSH disables password login | 3 | `PasswordAuthentication no` is judged pass, otherwise warn |
| ssh | SSH changes default port 22 | 1 | Non-22 is judged pass, otherwise warn |
| ssh | SSH protocol version is 2 | 1 | Not configured or being `2` is judged pass, otherwise fail |
| password | No empty-password accounts | 3 | `/etc/shadow` password field empty means fail, list account names |
| password | Password maximum validity period ≤ 90 days | 2 | `PASS_MAX_DAYS` ≤ 90 is judged pass, otherwise warn |
| account | No redundant UID=0 accounts | 3 | UID=0 other than root means fail, list account names |
| account | No login-capable passwordless accounts | 2 | Has login shell and shadow empty/locked means fail |
| file_perm | `/etc/shadow` permission security | 2 | Overly permissive permissions are judged warn/fail |
| file_perm | `/etc/passwd` permission security | 2 | Same as above |
| file_perm | `/etc/ssh/sshd_config` permission security | 1 | Same as above |
| firewall | Firewall enabled | 3 | ufw/firewalld/iptables detected as active is judged pass, otherwise fail |
| firewall | No high-risk ports listening | 3 | 23/445/135/1433/3389 listening is judged warn, list ports |
| cron | No suspicious cron jobs | 3 | crontab command containing `curl`/`wget`/`base64 -d`/`/tmp/`/`/dev/shm` is judged warn (list at most 5) |

Total weight 32. Converted to 40 points according to the formula above.

---

## Appendix B: Scoring Example

A typical public-facing Ubuntu machine:

| Module | Measured | Score |
| --- | --- | --- |
| Baseline | 12 of 14 items pass, 2 items warn (SSH uses port 22, `PASS_MAX_DAYS` not set) | got = 32 − 1×0.5 − 2×0.5 = 30.5 → `int(40×30.5/32)` = **38** |
| Login | 2000 failures within 24h, hitting 3 brute-force targets | `20 − 5×3` = **5** |
| Firewall | ufw enabled | **15** |
| File integrity | 1 change within 24h (`/etc/hosts`) | `15 − 3` = **12** |
| Threat detection | 1 warning (executable process under `/tmp`) | `10 − 2` = **8** |
| **Total score** |  | **78 → Good** |

This score holds up under interpretation: the configuration is basically reasonable (baseline 38), but the machine is being brute-forced (login 5), and the deductions come mainly from real attacks rather than false positives. The remediation path is also clear—enable firewall source restrictions, deploy fail2ban or use the Agent to block, and the login score will come back.
