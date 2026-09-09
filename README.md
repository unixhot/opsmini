<div align="center">

# OpsMini

**AI-Powered Server Management Panel**

[English](README.md) · [简体中文](docs/README.zh-CN.md) · [繁體中文](docs/README.zh-TW.md) · [日本語](docs/README.ja.md) · [한국어](docs/README.ko.md) · [ไทย](docs/README.th.md) · [Deutsch](docs/README.de.md)

</div>

---


OpsMini is an AI-powered server management panel (similar to BaoTa / 1Panel), differentiated by built-in AI operations and a standard REST API for external integration.

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
make build          # build for current platform
make build-all      # cross-compile for Linux amd64/arm64

./dist/opsmini -config configs/config.yaml
# open http://localhost:8888  (default account: opsmini — initial password is written to .init_passwd)
```

### Documentation

- [Developer Guide](docs/developer-guide.md)
- [Backend Architecture](docs/backend-architecture.md)
- [Build & Deployment](docs/build-and-deploy.md)
- [Security Audit](docs/security-audit.md)

### License & Copyright

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
