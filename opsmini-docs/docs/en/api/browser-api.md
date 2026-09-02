# Panel API (/api/v1)

The panel API serves the browser UI and uses user-session JWT + RBAC.

## Authentication

- `POST /auth/login` returns `access` + `refresh` tokens
- Subsequent requests carry `Authorization: Bearer <access_token>`
- Refresh the access token with the refresh token when it expires
- Login is rate-limited; with 2FA enabled, login requires `POST /auth/mfa/verify` first

## Unified Response

```json
{ "code": 0, "message": "ok", "data": { } }
```

## Endpoints

### Auth & Users

| Method | Path | Description |
|--------|------|-------------|
| POST | `/auth/login` | Login (rate-limited) |
| POST | `/auth/refresh` | Refresh token |
| POST | `/auth/logout` | Logout |
| POST | `/auth/mfa/verify` | Verify MFA code at login |
| GET | `/auth/mfa/status` | Current user's MFA binding status |
| POST | `/auth/mfa/setup` | Generate MFA binding QR/secret |
| POST | `/auth/mfa/enable` | Verify and enable MFA |
| POST | `/auth/mfa/disable` | Unbind MFA |
| GET/PUT | `/profile` | Personal profile (nickname / avatar / email) |
| POST | `/profile/password` | Change password |
| GET/POST/PUT/DELETE | `/users` | User CRUD |
| GET/POST/PUT/DELETE | `/roles` | Role CRUD |
| GET | `/roles/groups` | Permission groups |
| GET | `/permissions` | Current user's permissions |

### Panel Settings

| Method | Path | Description |
|--------|------|-------------|
| GET | `/settings` | Read all non-sensitive settings (`settings.view`) |
| PUT | `/settings` | Batch-update settings (`settings.edit`) |

> Sensitive items (`jwt_secret`, `metrics_pass`) are not returned; write-protected items (`jwt_secret`) cannot be overwritten. See [Panel Settings](../configuration/panel-settings.md).

### Monitoring & Alerts

| Method | Path | Description |
|--------|------|-------------|
| GET | `/dashboard/overview` | Dashboard overview |
| GET | `/dashboard/metrics` | Dashboard metrics |
| GET | `/system/monitor` | Monitor summary |
| CRUD | `/alert-rules` | Alert rules |
| GET | `/alert-events` | Alert events |

### Notifications

| Method | Path | Description |
|--------|------|-------------|
| GET | `/notifications` | Notification list |
| GET | `/notifications/unread-count` | Unread count |
| PUT | `/notifications/read-all` | Mark all read |
| PUT | `/notifications/:id/read` | Mark one read |
| DELETE | `/notifications/:id` / `/notifications` | Delete one / clear all |

### Resources

| Method | Path | Description |
|--------|------|-------------|
| CRUD | `/websites` | Websites |
| CRUD | `/databases` | Databases |
| GET/POST | `/apps` `/app-categories` | App store & categories |
| GET/POST | `/containers` `/images` `/volumes` `/networks` | Docker resources |
| GET/POST | `/files` | Files |
| CRUD | `/cron-jobs` | Cron jobs |
| GET | `/logs` `/logs/tail` | Logs |

### System & Security

| Method | Path | Description |
|--------|------|-------------|
| GET | `/system/info` `/processes` `/ports` `/disks` `/network` `/users` `/groups` `/firewall` | System resources |
| GET/POST | `/security/...` | Host security (baseline / FIM / threat / firewall / login) |
| GET | `/audit-logs` `/access-logs` | Audit / access logs |

### AI & Integration

| Method | Path | Description |
|--------|------|-------------|
| POST | `/ai/chat` | AI chat (function calling) |
| POST | `/ai/chat/stream` | AI chat (streaming SSE) |
| GET/POST/PUT/DELETE | `/mcp` | MCP config |
| GET/POST/DELETE | `/skills` and more | AI skills (SkillHub search/install/upload) |

### Terminal

| Method | Path | Description |
|--------|------|-------------|
| GET | `/terminal` | WebSocket terminal (query-token auth) |
| GET | `/containers/:id/exec` | Container WebSocket terminal |

## Notes

- All write operations are guarded by RBAC permission points (e.g. `website.create`, `container.edit`, `settings.edit`).
- Critical operations are written to the audit log; authenticated requests to the access log.
