# One-Click Install

Use the official installer: `install.sh` on Linux, `install.ps1` on Windows (admin PowerShell).

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

```powershell
# Windows (admin PowerShell)
irm https://opsmini.com/install.ps1 | iex
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


### Windows Options (install.ps1)

| Option | Description | Default |
|--------|-------------|---------|
| `-InstallDir <path>` | Program directory | `C:\Program Files\OpsMini` |
| `-DataDir <path>` | Data directory (binary, config, database, logs) | `C:\ProgramData\OpsMini` |
| `-Port <port>` | Panel port | `8888` |
| `-Binary <path>` | Local binary (offline install) | downloaded from OSS |
| `-Version <ver>` | Pin a specific release | current release |
| `-ServerAddr` / `-ServerToken` | Register into OpsAnt | — |
| `-Uninstall` | Stop and remove the service (keeps data) | — |

```powershell
# Custom port and data directory
.\install.ps1 -Port 9999 -DataDir D:\opsmini-data

# Offline install
.\install.ps1 -Binary .\opsmini.exe

# Register into OpsAnt right after install
.\install.ps1 -ServerAddr 47.104.172.52:4507 -ServerToken <token>
```

> On Windows the script registers a Windows service named `OpsMini` (SCM) instead of systemd, also starting on boot. It self-elevates when not run as Administrator.


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
