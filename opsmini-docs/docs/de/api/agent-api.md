# Agent-API (/agent/v1)

Die Agent-API ist eine standardmäßige REST-Schnittstelle für **Maschinen / Drittsysteme** und dient der Integration durch Monitoring-Plattformen, Automatisierungsskripte und Orchestrierungstools.

## Unterschiede zur Panel-API

| Dimension | Panel-API `/api/v1` | Agent-API `/agent/v1` |
|------|-------------------|----------------------|
| Nutzer | Browser (Mensch) | Externe Systeme (Maschine) |
| Authentifizierung | Benutzersitzungs-JWT + RBAC | Agent-Token (Bearer) |
| Umfang | Sämtliche UI-Funktionen + Terminal-WS | Ressourcenabfrage + Befehlsausführung (keine UI / kein Terminal) |
| Design | Interaktionsorientiert | Automatisierungsorientiert (idempotent, wiederholbar) |

## Authentifizierung

1. Auf der Seite „Einstellungen → API-Token" des Panels ein Zugriffstoken erzeugen (das Token wird in der Datenbank persistiert, nicht mehr in `config.yaml` geschrieben)
2. Anfrage mit `Authorization: Bearer <token>` senden
3. Die Middleware prüft das Token in konstanter Zeit; ein leeres Token deaktiviert die Agent-API

## Endpunkt-Übersicht

| Methode | Pfad | Beschreibung |
|------|------|------|
| GET | `/agent/v1/health` | Health-Check (Liveness-Probe) |
| GET | `/agent/v1/version` | Versionsinformation |
| GET | `/agent/v1/status` | Statuszusammenfassung: cpu / mem / Festplatte / Online-Dienste |
| GET | `/agent/v1/system/info` | Host-Informationen (hostname / os / Kernel) |
| GET | `/agent/v1/system/processes` | Prozessliste |
| GET | `/agent/v1/system/ports` | Lauschstatus der Ports |
| GET | `/agent/v1/system/disks` | Festplatte / Einhängepunkte |
| GET | `/agent/v1/websites` | Website-Liste |
| GET | `/agent/v1/databases` | Datenbankliste |
| GET | `/agent/v1/cron-jobs` | Geplante Aufgaben |
| GET | `/agent/v1/containers` | Container-Liste |
| POST | `/agent/v1/commands` | Befehl ausführen (Whitelist) |
| POST | `/agent/v1/script/run` | Skript ausführen |
| POST | `/agent/v1/file/upload` | Datei hochladen |

## Befehlsausführung & Whitelist

`/commands` erlaubt ausschließlich die Ausführung explizit autorisierter Befehlspräfixe; alles wird auditiert. Die Whitelist wird in `agent.allowed_commands` in `config.yaml` gepflegt:

```yaml
agent:
  allowed_commands:
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

## Aufrufbeispiel

```bash
curl -H "Authorization: Bearer <token>" \
     https://<host>:8888/agent/v1/status
```

## Sicherheitsempfehlungen

- HTTPS für die Übertragung verwenden
- In Hochsicherheitsszenarien die Quelle per IP-Whitelist einschränken
- Befehls-Whitelist minimieren, nur notwendige Befehle autorisieren
- Nach einem Token-Leck sofort auf der Seite „Einstellungen → API-Token" neu erzeugen
