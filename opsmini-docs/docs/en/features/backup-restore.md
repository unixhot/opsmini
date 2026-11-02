# Backup & Restore

## "One click" is not the same for both directions

| Action | One-click-ness | Why |
|---|---|---|
| Backup | ✅ genuinely one click, and schedulable | reads sources, writes a target, retry on failure, nothing destructive |
| Restore | ⚠️ "one click" with three gates | it overwrites production data, so it needs preflight, a pre-restore snapshot and automatic rollback |

Making restore "click and overwrite" would hand the user a delete button with no undo. So the UI calls it **one-click restore (with preflight and automatic rollback)**.

## What gets backed up

Three object kinds, one per line in "Backup objects":

| Syntax | Meaning |
|---|---|
| `/var/www` | a directory (or single file), archived as `tar.gz` |
| `mysql:opsmini` | a database, exported with `mysqldump` and compressed |
| `redis:/var/lib/redis/dump.rdb` | a Redis snapshot file, archived directly |

MySQL uses the connection already configured in the panel (local socket) with `--single-transaction --routines --triggers`, giving a consistent InnoDB snapshot without locking tables.

**Not supported**: PostgreSQL, Docker volumes, block/filesystem snapshots, incremental backups, backup encryption. All are candidates for later versions; none is promised here.

## What a snapshot looks like

```
<data dir>/data/backups/web-20261029-020000/
├── manifest.json                 # metadata: objects, sizes, sha256, hostname, version
└── data/
    ├── var_www.tar.gz
    ├── opsmini.sql.gz
    └── redis.rdb.gz
```

Directory mode `0700`, files `0600`. `manifest.json` records the sha256 of every archive, verified during restore.

Repository-style tools (restic/borg) are deliberately not used: dedupe, encryption and snapshot chains are powerful, but they add 15–25MB of dependencies and require the same tool to restore. This is "self-contained archives + a manifest" — any machine that can untar can restore.

## Backup flow

```
Validate every object (path exists, database name non-empty, redis path readable)
   ↓  validate everything before touching anything: otherwise a typo in the
      second of five objects fails after four archives were already written,
      leaving a half-finished snapshot
Execute one by one (serially, to avoid saturating IO)
   ↓  each object writes its manifest entry as it completes; a failure never
      leaves a "half backup"
Compute the total checksum → prune old snapshots by retention → write backup history
```

Retention defaults to 7; `0` keeps everything. Pruning deletes the oldest **whole directory**, never a single archive (that would invalidate the manifest).

## Restore flow (three gates)

**① Preflight**

- the snapshot name must be typed as confirmation (prevents mis-selection)
- target parent directory exists and is writable
- `mysql` client available (for database restores)
- snapshot files present and sha256 matching the manifest

**② Automatic pre-restore snapshot**

The current state is backed up first (directories via the same archiving logic, databases via dump), so a failure can return here.

**③ Execute and roll back**

- restore → health checks
- any failed check → **automatic rollback to the step ② snapshot** → a critical incident is raised in the event center

Health checks: directory file count plus spot-checked checksums, MySQL `SELECT 1` with key table row counts, Redis `PING` and keyspace comparison.

Rollback reliability comes from same-filesystem `rename` being atomic: before restoring, the target directory is `mv`'d to `xxx.pre-restore-<timestamp>`; on failure it is `mv`'d back.

## Panel operations

**Panel settings → Backup & restore**:

| Area | Content |
|---|---|
| Backup objects | one per line: directory / `mysql:db` / `redis:path` |
| Retention | default 7, 0 means never prune |
| Back up now | runs once and refreshes the snapshot list |
| Snapshots | name, time, size, object count, preflight result |
| Restore | type the snapshot name to confirm, returns preflight details and outcome |
| History | the result of every run, including failures |

## Scheduled backups

Backup is the **fourth kind of scheduled job** (alongside panel jobs, system crontab, user crontab), reusing the same scheduler, enable/disable and failure notification.

Configure in **Scheduled jobs → New → type "Backup job"**:

| Field | Notes |
|---|---|
| Backup objects | same line syntax as the backup page |
| Retention | pruning policy for old snapshots |
| Schedule | cron expression with presets (e.g. daily 02:00) |
| Notify | panel / email / WeCom / DingTalk / Feishu |
| Keep local copy | when using a remote target, keep the local snapshot after a successful upload |

It uses the **same engine** as "Back up now", producing identical artefacts — there is no "manual restores work but scheduled ones don't" situation.

Failures are written back to the job (last status / last error / consecutive failures), recorded in backup history, and picked up by the built-in "backup failures" alert rule.

## Remote targets: code ready, not enabled in this version

`internal/backup` contains S3-compatible (OSS / MinIO / AWS, self-written SigV4) and SFTP remote stores, but **v1.4.0 does not deliver them**:

- the panel does not guide you to configure a remote target
- the docs do not recommend production use
- no real-host interoperability testing was done (only unit tests and S3 signature test vectors)

To use off-host storage, add targets in `config.yaml` — no code change needed:

```yaml
backup:
  targets:
    - name: oss-bj
      type: s3
      endpoint: oss-cn-beijing.aliyuncs.com
      bucket: my-backup
      region: cn-beijing
      access_key: "..."
      secret_key: "..."
```

Once configured, the backup page offers a remote target selector and lets you browse and fetch remote snapshots. Upload happens after the local snapshot is fully written, so a failed upload never leaves a partial backup on the remote side.

**Note**: this path has not been tested against a real remote; verify a full round trip (upload → list → restore to a temporary directory) on a non-production path before relying on it.

## Related

| Module | Relationship |
|---|---|
| [Autonomous agent](autonomous-agent.md) | Restore failures raise critical incidents; backup failures can be caught by alert rules |
| [Change awareness](change-awareness.md) | Editing backup configuration is itself a change |
| [Host security](security.md) | "Are backups available" and "are they off-host" can count towards the security score |
| [Log pipeline](log-pipeline.md) | Independent of each other |

## Known limitations

- Large directory backups consume IO for their duration; schedule them off-peak (default 02:00)
- MySQL is backed up over the local socket; remote database instances are not supported
- Restore is overwrite-style: it restores only the objects in the snapshot and does not remove files created after it
- Snapshots are not encrypted; protect the data directory and the backup medium if it contains sensitive data
