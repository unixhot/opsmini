<div align="center">

# OpsMini

**KI-gesteuertes Server-Verwaltungspanel**

[English](../README.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [ไทย](README.th.md) · [Deutsch](README.de.md)

</div>

---

## Deutsch

OpsMini ist ein KI-gesteuertes Server-Verwaltungspanel (vergleichbar mit BaoTa / 1Panel), das sich durch integrierten KI-Betrieb und eine standardisierte REST API für externe Integration auszeichnet.

### Funktionen

- **KI-gestützter Betrieb** — Diagnose in natürlicher Sprache, Log-Analyse, Befehlsausführung
- **Standard-REST-API** — `/api/v1` (Browser-UI) und `/agent/v1` (Machine-to-Machine)
- **Docker-Verwaltung** — Container, Images, Volumes, Netzwerke
- **Web-Terminal** — SSH-ähnliche interaktive Shell über WebSocket + pty
- **i18n in 7 Sprachen** — Chinesisch (vereinfacht/traditionell), Englisch, Japanisch, Koreanisch, Thailändisch, Deutsch
- **Einzelnes Binary** — Frontend per `go:embed` eingebettet, keine Laufzeitabhängigkeiten

### Tech-Stack

Go · Gin · GORM · SQLite (reines Go) · Vue 3 · ECharts

### Schnellstart

```bash
make build          # Build für die aktuelle Plattform
make build-all      # Cross-Compile für Linux amd64/arm64

./dist/opsmini -config configs/config.yaml
# http://localhost:8888 öffnen (Standardkonto: opsmini, Passwort im Startprotokoll)
```

### Dokumentation

- [Entwicklerhandbuch](docs/developer-guide.md)
- [Backend-Architektur](docs/backend-architecture.md)
- [Build & Deployment](docs/build-and-deploy.md)
- [Host-Sicherheitsdesign](docs/security-audit.md)

### Lizenz & Urheberrecht

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
