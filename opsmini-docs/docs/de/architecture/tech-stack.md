# Technologie-Stack

## Auswahlübersicht

| Schicht | Auswahl | Begründung |
|----|------|------|
| Sprache | Go 1.25+ | einzelnes Binary, Cross-Kompilierung, gute Nebenläufigkeit, ausgereiftes Ökosystem |
| Web-Framework | Gin | größtes Ökosystem, reichhaltige Middleware |
| Datenbank | SQLite | eingebettet, wartungsfrei, passt zum Einzel-Server-Szenario |
| SQLite-Treiber | `glebarez/sqlite` (reines Go) | kein CGO, unkomplizierte Cross-Kompilierung |
| ORM | GORM | hohe Entwicklungseffizienz, bei komplexen Abfragen Rückgriff auf natives SQL |
| Echtzeitkommunikation | gorilla/websocket | Terminal, Log-tail, Metrik-Push |
| Aufgabenplanung | robfig/cron | geplante Panel-Aufgaben |
| Systemmonitoring | gopsutil | plattformübergreifend CPU / Speicher / Festplatte / Prozesse |
| Docker | offizielles SDK | Container / Images / Volumes / Netzwerke |
| Authentifizierung | JWT + refresh | zustandslose API + optionale Sitzung |
| 2FA | TOTP (RFC 6238) | zweite Verifizierung bei der Anmeldung |
| AI | abstrahierte LLM-Schnittstelle | einheitliche Anpassung von OpenAI / DeepSeek / Qwen / Ollama |
| Frontend | Vue 3 + ECharts | moderne SPA + Diagramme |

## Gesamtarchitektur

```
┌──────────────────────────────────────────────────────────┐
│              OpsMini (Einzel-Server-Panel, einzelnes Binary) │
│                                                          │
│   Vue3-SPA (Build-Ergebnis in das Binary eingebettet)      │
│        │  HTTP / WebSocket                                │
│   ┌────▼───────────────────────────────────────────────┐  │
│   │  Gin-Router → Middleware (Auth/Berechtigung/Audit/  │  │
│   │               Begrenzung/Sicherheitseingang)        │  │
│   │  Controller → Service → Repository                 │  │
│   └────┬──────────────────────────────────────────────┘  │
│        │                                                  │
│   ┌────▼───────────────────────────────────────────────┐  │
│   │  SQLite (Geschäftsdaten) │  Systemressourcen-       │  │
│   │  users/cron/...          │  Adapterschicht           │  │
│   │                          │  Docker SDK / crontab /   │  │
│   │                          │  gopsutil / Dateisystem / │  │
│   │                          │  SSH                      │  │
│   └──────────────────────────┴────────────────────────┘  │
│                                                          │
│   Zwei REST-APIs nach außen:                             │
│   · /api/v1   Panel-API (JWT + RBAC)                     │
│   · /agent/v1 Agent-API (Token + Befehls-Whitelist)      │
└──────────────────────────────────────────────────────────┘
```
