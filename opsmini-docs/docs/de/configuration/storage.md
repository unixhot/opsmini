# Datenspeicherung

OpsMini verwendet eingebettetes SQLite zur Speicherung sämtlicher Geschäftsdaten (Benutzer, Aufgaben, Alarme, Websites, Audit-Logs usw.).

## Datendatei

- Standardpfad: bei Ein-Klick-Installation `/data/opsmini/opsmini.db` (durch `database.path` in `config.yaml` festgelegt, standardmäßig relativ zum Verzeichnis der Konfigurationsdatei)
- Beim ersten Start werden Datei und Tabellen automatisch erzeugt (automatische Migration durch GORM)
- Sensible Felder (Schlüssel / Token / TOTP-Secret) werden mit AES-GCM verschlüsselt gespeichert

## Sicherung

SQLite ist eine einzelne Datei — direktes Kopieren genügt als Sicherung:

```bash
# Empfohlen: Dienst vor der Sicherung stoppen, um Konsistenz zu gewährleisten
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /backup/opsmini.db.$(date +%s)
sudo systemctl start opsmini
```

## Wiederherstellung

```bash
sudo systemctl stop opsmini
cp /backup/opsmini.db.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```

## Überblick über gespeicherte Inhalte

| Daten | Tabelle | Beschreibung |
|------|-----|------|
| Benutzer & Rollen | `users` / `roles` | Konten, Passwort-Hashes, RBAC-Rollen und -Berechtigungen |
| Sitzungen | `sessions` | Refresh-Token und Gültigkeit |
| Panel-Konfiguration | `settings` | KV wie Design / Sprache / Menü-Sichtbarkeit |
| Websites / Datenbanken | `websites` / `databases` | Datensätze von Sites und Datenbankinstanzen |
| Geplante Aufgaben | `cron_jobs` | Geplante Aufgaben des Panels |
| Alarmregeln / -ereignisse | `alert_rules` / `alert_events` | Monitoring-Alarme |
| Audit- / Zugriffslogs | `audit_logs` / `access_logs` | Betriebs- und Zugriffsaufzeichnungen (standardmäßig 7 Tage Aufbewahrung) |

## Datenbereinigung

- Audit- und Zugriffslogs werden standardmäßig 7 Tage aufbewahrt und stündlich automatisch bereinigt; die Aufbewahrungstage lassen sich in den Panel-Einstellungen anpassen
- Alarmereignisse werden nach Ablauf der Aufbewahrungszeit automatisch bereinigt (standardmäßig 30 Tage)
