# 5-Minute Install

Use the official `install.sh` to deploy OpsMini on Linux in one shot.

## Prerequisites

- Host: Linux `x86_64` or `aarch64`
- `sudo` privileges
- Internet access (to download the binary) or a locally prepared binary

## One-Click Install

```bash
# One-line install (recommended, auto-downloads the binary from the official site)
curl -fsSL https://opsmini.com/install.sh | sudo bash

# Option 1: download and install from a URL (GitHub Release)
sudo ./install.sh -u https://example.com/opsmini-v1.0.0-linux-amd64.tar.gz

# Option 2: install from a local binary (defaults to /data/opsmini, port 8888)
sudo ./install.sh -b ./opsmini

# Custom directory and port
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## Options

| Option | Description | Default |
|--------|-------------|---------|
| `-d, --dir <path>` | Install directory | `/data/opsmini` |
| `-p, --port <port>` | Panel port | `8888` |
| `-b, --binary <path>` | Binary path | auto-detect |
| `-u, --url <url>` | Download from URL | — |
| `-n, --no-systemd` | Skip systemd registration | — |

## First Login

The startup log prints something like:

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

- Open `http://<host>:8888`
- Username: `opsmini`
- Password: in the startup log or `/data/opsmini/.init_passwd`

> ⚠️ Record the password immediately and change it after login.

## Next Steps

- [First Login](first-steps.md)
- [Manual Deployment](../installation/manual-install.md)
