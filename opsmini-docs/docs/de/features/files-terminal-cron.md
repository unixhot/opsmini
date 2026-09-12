# Dateien / Terminal / Geplante Aufgaben / Logs

## Dateiverwaltung

- Dateisystemverzeichnisse durchsuchen
- Dateien hochladen / herunterladen / umbenennen / löschen
- Neue Verzeichnisse erstellen

## Web-Terminal

- SSH-ähnliches interaktives Terminal auf Basis von WebSocket + pty
- Nur für Rollen ab `operator` freigeschaltet; Sitzungen werden auditierend aufgezeichnet

```text
Frontend ws://host/api/v1/terminal?token=...
  → Server prüft token + Rolle
  → pty starten → bidirektionale Datenweiterleitung
  → beim Schließen pty freigeben, Sitzungsdauer aufzeichnen
```

## Geplante Aufgaben

Drei Aufgabentypen:

| Typ | Beschreibung | Operation |
|------|------|------|
| Panel-Aufgaben | Dauerhaft vom Panel geplant (robfig/cron) | Erstellen/Lesen/Ändern/Löschen |
| Systemaufgaben | `/etc/crontab`, `/etc/cron.d/` | Nur-Lesen-Anzeige + kontrolliertes Bearbeiten |
| Benutzeraufgaben | `/var/spool/cron/<user>` | Nur-Lesen-Anzeige + kontrolliertes Bearbeiten |

## Logs

- Liste der Panel-Logs ansehen
- Logs in Echtzeit per tail verfolgen
- Strukturierte Logs (zerolog) mit Level-Einteilung
