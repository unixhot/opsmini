<div align="center">

# OpsMini

**The AI ops agent that lives on your server**

[English](README.en.md) · [简体中文](../README.md) · [繁體中文](README.zh-TW.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [ไทย](README.th.md) · [Deutsch](README.de.md)

</div>

---


OpsMini is a resident AI ops agent on your Linux host: it collects metrics, judges what is wrong and acts — pausing for your approval before anything risky. The web UI is just its window: you can click nothing at all and it still gets the work done and leaves a report. The whole agent is one static binary of a few dozen MB, and all data stays on the machine.

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

# For Ops

curl -fsSL https://opsmini.com/install.sh | sudo bash

# For Dev

make build          # build for current platform
make build-all      # cross-compile for Linux amd64/arm64

./dist/opsmini -config configs/config.yaml
# open http://localhost:8888  (default account: opsmini — initial password is written to .init_passwd)
```

### Documentation

- [Developer Guide](developer-guide.md)
- [Backend Architecture](backend-architecture.md)
- [Build & Deployment](build-and-deploy.md)
- [Security Audit](security-audit.md)

### License & Copyright

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
