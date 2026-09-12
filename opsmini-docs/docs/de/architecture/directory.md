# Verzeichnisstruktur

## Schichten

Innerhalb jedes Moduls gelten strikt drei Schichten:

```
Controller (HTTP-Handler): Parameter parsen und prüfen, Service aufrufen, Antwort zusammenstellen
    → Service (Geschäftslogik): Repository und Systemressourcen orchestrieren, Transaktionsgrenzen
    → Repository (GORM): reiner Datenzugriff, keine Geschäftslogik
```

## Verzeichnisorganisation

```
opsmini/
├── cmd/
│   └── agent/main.go          # Einstiegspunkt des Hauptprogramms
├── internal/
│   ├── router/                # Routenregistrierung, API-Versionen
│   ├── middleware/            # Authentifizierung, Berechtigung, Audit, Begrenzung, Sicherheitseingang
│   ├── api/v1/                # Controller (paketiert nach Modul)
│   ├── service/               # Service (paketiert nach Modul)
│   ├── repository/            # Repository (paketiert nach Modul)
│   ├── model/                 # GORM-Datenmodelle
│   ├── config/                # Konfigurationsladen
│   └── pkg/                   # Allgemeine Hilfsmittel (jwt/response/store)
├── web/                       # Vue3-Frontend (nach dem Build eingebettet)
│   ├── index.html             # SPA-Einstieg
│   └── static/                # Statische Bibliotheken wie echarts/vue/xterm
├── configs/                   # Beispiel für Standardkonfiguration
├── docs/                      # Dokumentation
└── install/                   # Ein-Klick-Installationsskript
```

## Schlüsselmodule

| Modul | Zuständigkeit |
|------|------|
| `auth` | Anmelden / Abmelden, Sitzung, 2FA, RBAC |
| `setting` | Panel-Konfiguration, Design, Menü-Sichtbarkeit, Sprache |
| `dashboard` / `monitor` | Metrik-Aggregation, Zeitreihenerfassung, Alarmregeln |
| `website` / `database` / `appstore` | Websites, Datenbanken, App-Store |
| `container` | Docker-Container / Images / Volumes / Netzwerke |
| `system` / `file` / `terminal` | Systemressourcen, Dateien, Web-Terminal |
| `cron` / `log` | Geplante Aufgaben, Logs |
| `ai` | LLM-Anbindung, Kontext, Ausführung in natürlicher Sprache |
| `security` | Baseline, FIM, Bedrohung, Firewall, Anmeldesicherheit |
| `agent` | Externe REST-API (`/agent/v1`) |
