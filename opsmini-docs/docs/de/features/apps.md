# App-Verwaltung

Die App-Verwaltung vereint die vier Ressourcenarten Websites, Datenbanken, Container und App-Store.

## Websites

- Nginx-Sites und statische Sites erstellen / verwalten
- Domäne, Laufzeitumgebung (PHP / Node / statisch), Ausstellung und Verlängerung von SSL-Zertifikaten konfigurieren
- Sites starten/stoppen und Status anzeigen

## Datenbanken

- MySQL-, PostgreSQL-Datenbankinstanzen erstellen / verwalten
- Datenbankliste, Zeichensatz und weitere Konfigurationen anzeigen

## Container (Docker)

- **Container**: auflisten, starten, stoppen, neu starten, löschen
- **Images**: anzeigen und löschen
- **Volumes**: anzeigen und löschen
- **Netzwerke**: anzeigen und löschen

> Die Docker-Verwaltung erfordert, dass Docker auf dem Host installiert und `DOCKER_HOST` korrekt konfiguriert ist. Schlägt die Initialisierung des Docker-Clients fehl, degradiert das Panel anmutig und blendet die Container-Routen aus.

## App-Store

- Integrierte Kategorien gängiger Anwendungen (Webdienste, Datenbanken, Cache, Monitoring usw.)
- Ein-Klick-Installation / Deinstallation / Start / Stopp / Neustart von Anwendungen (Docker-Shell-Skripte)
- Unterstützt den Import benutzerdefinierter Anwendungsvorlagen und die Synchronisierung offizieller Vorlagen
