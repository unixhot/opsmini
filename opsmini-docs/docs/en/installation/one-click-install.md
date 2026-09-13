# One-Click Install

Use the official `install.sh` to deploy OpsMini on Linux in one shot.

## Quick Start

```bash
# One-line install (recommended, auto-downloads the binary from Alibaba Cloud OSS)
curl -fsSL https://opsmini.com/install.sh | sudo bash

# Install from a local binary (defaults to /data/opsmini, port 8888)
sudo ./install.sh -b ./opsmini

# Download and install from a URL (raw binary or .tar.gz)
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# Custom directory and port
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-d, --dir <path>` | Install directory | `/data/opsmini` |
| `-p, --port <port>` | Panel port | `8888` |
| `-b, --binary <path>` | Binary path | auto-download from OSS |
| `-u, --url <url>` | Download from URL (raw binary or `.tar.gz`) | official OSS URL |
| `-n, --no-systemd` | Skip systemd registration | — |
| `-h, --help` | Show help | — |

## What It Does

1. Creates the install directory (default `/data/opsmini`)
2. Copies / downloads the binary to `<dir>/opsmini` (from Alibaba Cloud OSS by architecture)
3. Generates `<dir>/config.yaml` (port, SQLite path, JWT TTL, log level & log file; the JWT secret and Agent token are auto-generated on first start and stored in the database)
4. Generates a 16-char random admin password written to `<dir>/.init_passwd` (mode 600)
   - **Fresh install**: injected via the `OPSMINI_INIT_PASSWORD` env var on first start
   - **Reinstall** (database already exists): resets the password via `-reset-pass`, so the printed password is the one that actually works
5. Registers and starts the `opsmini.service` systemd service
6. Probes the service and prints the URL / username / password / log file path

## Resulting Layout

```
/data/opsmini/
├── opsmini              # binary
├── config.yaml          # config (600)
├── opsmini.db           # SQLite database (created on first start)
├── opsmini.log          # runtime log file
└── .init_passwd         # initial password (600)
```

## Binary Distribution

- **Official (Alibaba Cloud OSS)**: `https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v<version>-linux-<arch>`, downloaded by default
- **Local development**: `make build-all` in the repo root produces `dist/opsmini-<ver>-linux-amd64` / `-arm64`

## Account Recovery {: #account-recovery }

To reset a forgotten password or lost MFA, run on the server (stop the service first):

```bash
# Reset password
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-pass opsmini
systemctl start opsmini

# Clear MFA binding
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-mfa opsmini
systemctl start opsmini
```

> `-reset-mfa` / `-reset-pass` operate directly on SQLite and exit without starting the web service. Ensure the service is stopped.
