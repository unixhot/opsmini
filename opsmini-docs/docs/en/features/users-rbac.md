# Users & Permissions

OpsMini uses role-based access control (RBAC) to finely manage panel operation permissions.

## User Management

- Create / edit / disable / delete users
- View last login time
- Each user can bind an independent MFA (TOTP)
- The personal settings page supports changing nickname, avatar, email, and password

## Roles & Permissions

Three built-in roles:

| Role | Permission scope |
|------|----------|
| `admin` | All permissions (including users / roles / settings) |
| `operator` | Daily operations (apps, containers, files, terminal, scheduled tasks, etc.) |
| `readonly` | Read-only view |

Custom roles are supported, with fine-grained assignment by **permission point**, for example:

- `user.create` / `user.edit` / `user.delete`
- `website.create` / `website.edit` / `website.delete`
- `container.edit` / `container.delete`
- `security.scan` / `security.firewall` / `security.fim` / `security.threat`
- `settings.view` / `settings.edit` (panel settings)
- `apps.install`, `cron.create`, `database.create`, `file.write`, `mcp.manage`, `skill.manage`, `alert.manage`, etc.

## Two-Factor Authentication (2FA) {: #2fa }

- **Global toggle**: after enabling Panel "Settings → Two-step verification", all accounts with a bound authenticator must enter a 6-digit dynamic code when logging in
- **Per-user binding**: users generate a QR code in personal security settings and scan it with Google Authenticator / 1Password / various cloud-provider authenticator apps to bind
- **Account recovery**: when the authenticator is lost, log into the server and run `opsmini -reset-mfa <username>` to reset that user's MFA binding

## Security Design

- Passwords stored as bcrypt hashes
- Short-lived access JWT (15 minutes) + revocable refresh token (7 days)
- The JWT signing secret is automatically generated on first startup and persisted to the database, not written to the configuration file
- Login failure rate limiting and lockout (anti-brute-force)
- Critical operations are written to the audit log
- Forgotten passwords can be reset by logging into the server and running `opsmini -reset-pass <username>`
