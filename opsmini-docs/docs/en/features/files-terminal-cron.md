# Files / Terminal / Scheduled Tasks / Logs

## File Management

- Browse filesystem directories
- Upload / download / rename / delete files
- Create directories

## Web Terminal

- An SSH-like interactive terminal based on WebSocket + pty
- Only available to operator and above roles; sessions are audited

```text
Frontend ws://host/api/v1/terminal?token=...
  → Server validates token + role
  → Starts pty → bidirectional data forwarding
  → On close, recycles pty and records session duration
```

## Scheduled Tasks

Three types of tasks:

| Type | Description | Operation |
|------|------|------|
| Panel tasks | Scheduled by the panel's resident scheduler (robfig/cron) | Create, read, update, delete |
| System tasks | `/etc/crontab`, `/etc/cron.d/` | Read-only display + controlled editing |
| User tasks | `/var/spool/cron/<user>` | Read-only display + controlled editing |

## Logs

- View the panel log list
- Real-time log tail
- Structured logs (zerolog) with levels
