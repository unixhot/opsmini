# Requirements

## Target Host

| Item | Minimum | Recommended |
|------|---------|-------------|
| OS | Mainstream Linux (x86_64 / aarch64) | Ubuntu 20.04+ / Debian 11+ / CentOS 7+ |
| CPU | 1 core | 2 cores |
| Memory | 512 MB | 1 GB+ |
| Disk | 200 MB (binary + data) | 1 GB+ (logs + SQLite) |
| Network | Internet (first download) | Stable intranet |

## Architecture Support

| Platform | Purpose |
|----------|---------|
| `linux/amd64` | Mainstream x86_64 servers (production) |
| `linux/arm64` | ARM servers (Graviton / Raspberry Pi / Kunpeng / Phytium) |
| `darwin/amd64` / `darwin/arm64` | macOS dev machines (development only) |

## Runtime Dependencies

- **Zero runtime dependencies**: frontend, SQLite, and static assets are embedded in the binary.
- Docker management requires Docker on the host (the panel itself does not depend on it).

## Privileges

- The install script requires `root` or `sudo`.
- Run the panel under a dedicated low-privilege user (e.g. `opsmini`), see [systemd](systemd.md).
