# Data Storage

OpsMini stores all business data (users, tasks, alerts, websites, audit logs) in embedded SQLite.

## Data File

- Default path: `opsmini.db` (next to the config file, or `database.path` in `config.yaml`)
- Created and migrated automatically on first start (GORM)
- Sensitive fields (secrets / tokens / TOTP secret) are encrypted with AES-GCM at rest

## Backup

SQLite is a single file — copy it to back up:

```bash
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /backup/opsmini.db.$(date +%s)
sudo systemctl start opsmini
```

## Restore

```bash
sudo systemctl stop opsmini
cp /backup/opsmini.db.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```

## What's Stored

| Data | Table | Notes |
|------|-------|-------|
| Users & roles | `users` / `roles` | Accounts, password hashes, RBAC |
| Sessions | `sessions` | Refresh tokens and expiry |
| Panel settings | `settings` | Theme / language / menu visibility KV |
| Websites / databases | `websites` / `databases` | Site and database records |
| Cron jobs | `cron_jobs` | Panel cron jobs |
| Alert rules / events | `alert_rules` / `alert_events` | Monitoring alerts |
| Audit / access logs | `audit_logs` / `access_logs` | Kept 7 days by default |

## Retention

- Audit and access logs default to 7 days, cleaned hourly; adjustable in panel settings.
- Alert events are cleaned by retention time (default 30 days).
