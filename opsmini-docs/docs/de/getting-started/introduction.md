# Produktvorstellung

OpsMini ist ein leichtgewichtiges Linux-Server-Verwaltungspanel (vergleichbar mit 宝塔 / 1Panel) mit den beiden zentralen Alleinstellungsmerkmalen: integrierte AI-LLM-Betriebsführung und standardmäßige REST API für externe Integration.

## Positionierung

| Dimension | Beschreibung |
|------|------|
| Ziel-Szenario | Ein einzelner Linux-Server (läuft auch auf kleinen 1C1G-Hosts) |
| Kernphilosophie | Eine Maschine installieren, eine Maschine verwalten — leichtgewichtig, Einzel-Server zuerst, integrierbar |
| Alleinstellungsmerkmale | AI-LLM-Betriebsführung + Standard-REST-API |

## Kernfunktionen

- **AI-LLM-Betriebsführung**: Fehler in natürlicher Sprache diagnostizieren, Logs analysieren, Betriebsbefehle ausführen, Anbindung an OpenAI / DeepSeek / Qwen / Ollama
- **Standard-REST-API**: `/api/v1` (Browser-UI) und `/agent/v1` (Machine-to-Machine) für die Integration durch Monitoring-Plattformen, Automatisierungsskripte und Orchestrierungstools
- **Docker-Verwaltung**: die vier Ressourcenarten Container, Images, Volumes und Netzwerke
- **Web-Terminal**: SSH-ähnliches interaktives Terminal auf Basis von WebSocket + pty
- **Host-Sicherheit**: Baseline-Prüfung, File-Integrity-Monitoring (FIM), Bedrohungserkennung, Firewall, Anmeldesicherheit
- **i18n in 7 Sprachen**: vereinfachtes/traditionelles Chinesisch, Englisch, Japanisch, Koreanisch, Thailändisch, Deutsch
- **Einzel-Binary-Auslieferung**: Frontend per `go:embed` eingebettet, null Laufzeitabhängigkeiten

## Technologie-Stack

Go 1.25 · Gin · GORM · SQLite (reines Go) · Vue 3 · ECharts

## Unterschiede zu vergleichbaren Produkten

| Fähigkeit | 宝塔 / 1Panel | OpsMini |
|------|--------------|---------|
| Betriebsart | Grafische manuelle Bedienung | Grafisch + AI-Betrieb in natürlicher Sprache |
| Externe Integration | Keine Standard-API | Standard-REST-API (`/agent/v1`) |
| Monitoring-Integration | Erfordert zusätzliche node_exporter-Installation | Integriertes `/metrics` (node_exporter-kompatibel) |
| Auslieferungsform | Installationsskript + mehrere Komponenten | Einzelnes statisch gelinktes Binary |

## Open-Source-Lizenz

Apache License 2.0 — frei nutzbar, veränderbar und weiterverteilbar.
