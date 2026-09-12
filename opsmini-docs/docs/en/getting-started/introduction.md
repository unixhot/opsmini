# Introduction

OpsMini is a lightweight Linux host operations panel (benchmarked against BaoTa / 1Panel), with built-in AI large-model operations and an external standard REST API as its core differentiators.

## Positioning

| Dimension | Description |
|------|------|
| Target scenario | A single Linux server (runs even on a 1C1G small host) |
| Core philosophy | Install one, manage one; lightweight, single-machine first, integrable |
| Differentiators | AI large-model operations + standard REST API |

## Core Features

- **AI large-model operations**: diagnose faults in natural language, analyze logs, execute operations commands, integrating OpenAI / DeepSeek / Qwen / Ollama
- **Standard REST API**: `/api/v1` (browser UI) and `/agent/v1` (machine-to-machine), for integration by monitoring platforms, automation scripts, and orchestration tools
- **Docker management**: containers, images, volumes, and networks — four types of resources
- **Web terminal**: an SSH-like interactive terminal built with WebSocket + pty
- **Host security**: baseline checks, File Integrity Monitoring (FIM), threat detection, firewall, login security
- **7-language i18n**: Simplified/Traditional Chinese, English, Japanese, Korean, Thai, German
- **Single-binary delivery**: the frontend is embedded via `go:embed`, zero runtime dependencies

## Tech Stack

Go 1.25 · Gin · GORM · SQLite (pure Go) · Vue 3 · ECharts

## Differences from Similar Products

| Capability | BaoTa / 1Panel | OpsMini |
|------|--------------|---------|
| Operations method | Graphical manual operation | Graphical + AI natural-language operations |
| External integration | No standard API | Standard REST API (`/agent/v1`) |
| Monitoring integration | Requires installing node_exporter separately | Built-in `/metrics` (node_exporter compatible) |
| Delivery form | Install script + multiple components | Single statically linked binary |

## Open Source License

Apache License 2.0, free to use, modify, and distribute.
