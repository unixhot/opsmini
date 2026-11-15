<div align="center">

# OpsMini

**The AI ops agent that lives on your server**

[English](README.en.md) · [简体中文](../README.md) · [繁體中文](README.zh-TW.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [ไทย](README.th.md) · [Deutsch](README.de.md)

</div>

---


OpsMini is a resident AI ops agent for **Linux and Windows** hosts: it collects metrics, judges what is wrong and acts — pausing for your approval before anything risky. The web UI is just its window: you can click nothing at all and it still gets the work done and leaves a report. The whole agent is one static binary of a few dozen MB, and all data stays on the machine.

### Features

- **AI-powered operations** — natural-language diagnosis, log analysis, command execution
- **Standard REST API** — `/api/v1` (browser UI) and `/agent/v1` (machine-to-machine)
- **Docker management** — containers, images, volumes, networks
- **Web terminal** — SSH-like interactive shell over WebSocket + pty
- **7-language i18n** — Simplified/Traditional Chinese, English, Japanese, Korean, Thai, German
- **Single binary** — frontend embedded via `go:embed`, zero runtime dependencies

### Tech Stack

Go · Gin · GORM · SQLite (pure Go) · Vue 3 · ECharts

### Quick Start

```bash

# For Ops (Linux)

curl -fsSL https://opsmini.com/install.sh | sudo bash

# For Ops (Windows, elevated PowerShell)

iwr -useb https://opsmini.com/install.ps1 -OutFile $env:TEMP\opsmini.ps1; & $env:TEMP\opsmini.ps1

# For Dev

make build          # build for current platform
make build-all      # cross-compile for Linux amd64/arm64
# Windows targets use the same toolchain:
#   CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/opsmini.exe ./cmd/agent

./dist/opsmini -config configs/config.yaml
# open http://localhost:8888  (default account: opsmini — initial password is written to .init_passwd)
```

### Documentation

- [Developer Guide](developer-guide.md)
- [Backend Architecture](backend-architecture.md)
- [Build & Deployment](build-and-deploy.md)
- [Host Security Design](host-security-design.md)

### License & Copyright

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
