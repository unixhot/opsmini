# FAQ

## Installation & Startup

**Q: `go mod tidy` gets stuck or reports Bad Gateway?**

`proxy.golang.org` may be blocked under mainland China network conditions; run `export GOPROXY=https://goproxy.cn,direct`.

**Q: It says the Go version is too low?**

OpsMini requires Go 1.25+. Download the official precompiled binary and add it to PATH.

**Q: Visiting `/` returns 301 or a blank page?**

The frontend static resources are already embedded via `go:embed`; use the `make build` artifact and do not manually split the frontend directory.

## Accounts & Security

**Q: Forgot the admin password?**

Run `opsmini -config <path> -reset-pass opsmini` on the server to reset it to a random password (see [Installation & Deployment](../installation/index.md) for details).

**Q: Lost the MFA two-factor authentication code?**

Run `opsmini -config <path> -reset-mfa opsmini` to clear the MFA binding, then log in again and rebind.

## Deployment & Operations

**Q: How do I upgrade the version?**

Back up `opsmini.db` → replace the binary → restart the service. SQLite is automatically migrated by GORM, so there is usually no need to alter tables manually.

**Q: The web terminal cannot connect behind a reverse proxy?**

You need to enable the WebSocket upgrade headers in Nginx (`proxy_http_version 1.1` + `Upgrade`/`Connection`); see [Ports & Reverse Proxy](../configuration/ports-proxy.md) for details.

**Q: How do I make Prometheus monitor this machine?**

Enable `metrics` in the configuration, then Prometheus directly scrapes `http://<host>:8888/metrics`.
