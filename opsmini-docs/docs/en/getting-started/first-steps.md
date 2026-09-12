# First Login & Initialization

After installation is complete, follow these steps to finish the first login and security initialization.

## 1. Get the Initial Password

The startup log prints the initial account information:

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

The password can also be viewed in `/data/opsmini/.init_passwd` (permission 600) in the installation directory.

## 2. Log in to the Panel

1. Open `http://<host>:8888` in a browser
2. Enter the username `opsmini` and the initial password
3. After a successful login, you enter the dashboard

> ⚠️ In production, be sure to expose the panel via [reverse proxy + HTTPS](../configuration/https.md) to avoid plaintext transmission.

## 3. Change the Password

1. Go to "Personal Settings"
2. Set a new password under "Change Password" and save it

If you forget the password, log into the server and run `opsmini -reset-pass <username>` to reset it.

## 4. Bind Two-Factor Authentication (Recommended)

1. Enable global 2FA in Panel "Settings → Two-step verification"
2. Go to personal security settings and scan the QR code to bind a TOTP authenticator (Google Authenticator / 1Password, etc.)
3. Enter the dynamic code to complete binding

After binding, each login requires an additional 6-digit dynamic code, greatly improving account security. If you lose the authenticator, you can recover with `opsmini -reset-mfa <username>`.

## 5. Configure the Security Entry (Optional)

Set the security entry prefix in Panel "Settings → Basic Settings" (or directly modify `server.secret_entry` in `config.yaml`):

```yaml
server:
  secret_entry: "/opsmini_panel"
```

After configuration, the panel address becomes `http://<host>:8888/opsmini_panel`, which can hide the real entry together with a reverse proxy to prevent port scanning.

## 6. Next Steps

- [Configure the AI assistant](../configuration/panel-settings.md#ai-config)
- [Enable the external REST API](../api/agent-api.md)
- [Explore features](../features/index.md)
