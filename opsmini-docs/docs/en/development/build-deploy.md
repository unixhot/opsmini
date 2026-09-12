# Build & Deploy

## Prerequisites

- Go 1.25+ (the pure Go driver has no CGO and supports cross-compilation directly)

## Getting the Code

```bash
git clone <repo-url> opsmini
cd opsmini
export GOPROXY=https://goproxy.cn,direct   # China mirror
go mod tidy
```

## Build Commands

```bash
make build          # build for the current platform → dist/opsmini
make build-all      # cross-compile linux/darwin amd64/arm64
make clean          # clean dist/
make version        # print version info
```

## Version Injection

Build information is injected via `-ldflags -X main.version/-X main.buildTime/-X main.gitCommit`:

```bash
make build VERSION=v1.0.0
./dist/opsmini -version
# opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

## Artifact Naming

`opsmini-<version>-<os>-<arch>`, e.g. `opsmini-v1.0.0-linux-amd64`.

## Delivered Artifacts

| Artifact | Description |
|------|------|
| `opsmini` single binary | Backend API + frontend UI + SQLite, roughly 28~30MB statically linked |
| `configs/config.yaml` | Configuration template |
| `opsmini.db` | Generated automatically on first run |

> Deployment only requires copying the binary + configuration file; no other runtime dependencies.
