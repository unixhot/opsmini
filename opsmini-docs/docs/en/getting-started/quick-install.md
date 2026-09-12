# 5-Minute Install

Use the official `install.sh` to deploy OpsMini on Linux in one shot.

## Prerequisites

- Host: Linux `x86_64` or `aarch64`
- `sudo` privileges
- Internet access (to download the binary) or a locally prepared binary

## One-Click Install

```bash
# One-line install (recommended, auto-downloads the binary from Alibaba Cloud OSS)
curl -fsSL https://opsmini.com/install.sh | sudo bash

# Download and install from a URL
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# Install from a local binary (defaults to /data/opsmini, port 8888)
sudo ./install.sh -b ./opsmini

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

## First Login

- Open `http://<host>:8888`
- Username: `opsmini`
- Password: the password printed by the install script, or `/data/opsmini/.init_passwd`

> ⚠️ Record the password immediately and change it after login. Reinstalling resets the password and prints a new one.

## Next Steps

- [First Login](first-steps.md)
- [Manual Deployment](../installation/manual-install.md)
