# REST API

OpsMini stellt nach außen zwei standardmäßige REST-APIs bereit, die für die Integration durch die Browser-UI und externe Systeme aufgerufen werden können.

## API-Überblick

| API | Präfix | Nutzer | Authentifizierung |
|-----|------|--------|------|
| Panel-API | `/api/v1` | Browser (Mensch) | Benutzersitzungs-JWT + RBAC |
| Agent-API | `/agent/v1` | Externe Systeme (Maschine) | Agent-Token + Befehls-Whitelist |
| Prometheus-Metriken | `/metrics` | Monitoring-Systeme | Optionale Basic-Authentifizierung |

Einheitliches Antwortformat: `{ "code": 0, "message": "ok", "data": ... }`; `code` ungleich 0 bedeutet einen Geschäftsfehler.

<div class="grid cards" markdown>

-   :material-web: **[Panel-API (/api/v1)](browser-api.md)**

    ---

    Sämtliche UI-Fähigkeiten: Authentifizierung, Benutzer, Monitoring, Websites, Container, Dateien, AI u. a.

-   :material-api: **[Agent-API (/agent/v1)](agent-api.md)**

    ---

    Machine-to-Machine-Schnittstellen für Ressourcenverwaltung und Befehlsausführung.

-   :material-chart-line: **[Prometheus /metrics](prometheus.md)**

    ---

    node_exporter-kompatible Metriken, direktes Scrapen durch Prometheus.

</div>
