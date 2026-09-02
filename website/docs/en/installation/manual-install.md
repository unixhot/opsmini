# Manual Install (from Source)

Build from source and deploy manually when you don't want the install script.

## Prerequisites

| Dependency | Version | Notes |
|------------|---------|-------|
| Go | **1.25+** | Pure-Go driver, no CGO, cross-compiles on any platform |
| Memory | ≥ 512MB | Lightweight build and runtime |

> **Why no CGO toolchain**: the SQLite driver is `github.com/glebarez/sqlite` (pure Go), so `CGO_ENABLED=0` cross-compiles a fully static binary on any platform.

## Get the Code

```bash
git clone <repo-url> opsmini
cd opsmini

# Configure a Go proxy mirror if needed (e.g. China)
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

## Build

```bash
# Build for the current platform → dist/opsmini
make build

# Cross-compile all targets
make build-all
# Outputs:
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64

# Pin a version explicitly
make build VERSION=v1.0.0

# Check the binary version
./dist/opsmini -version
```

## Verify Architecture

```bash
file dist/*
# Expect: ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64, all statically linked
```

## Manual Deployment

### Directory Layout

```
/opt/opsmini/
├── opsmini              # binary
└── config.yaml          # config file
/var/lib/opsmini/
└── opsmini.db           # data (auto-created on first start)
```

### Place Binary and Config

```bash
sudo mkdir -p /opt/opsmini /var/lib/opsmini
sudo cp dist/opsmini-*-linux-amd64 /opt/opsmini/opsmini
sudo cp configs/config.yaml /opt/opsmini/config.yaml

# Update config: absolute database path (JWT signing secret needs no config — auto-generated on first start)
sudo sed -i 's|path: "opsmini.db"|path: "/var/lib/opsmini/opsmini.db"|' /opt/opsmini/config.yaml

sudo chmod +x /opt/opsmini/opsmini
```

### Run

```bash
/opt/opsmini/opsmini -config /opt/opsmini/config.yaml
```

The first start creates tables and the default admin account. Open `http://<host>:8888`.

## Next Steps

- [systemd](systemd.md)
- [Configuration](../configuration/config-file.md)
