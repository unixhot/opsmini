# Host-Sicherheit

OpsMini enthält ein integriertes Host-Sicherheitscenter mit fünf Sicherheitsfähigkeiten.

## Sicherheitsüberblick

Die Startseite des Sicherheitscenters zeigt die gesamte Sicherheitslage aggregiert an, darunter Baseline-Konformität, Dateiänderungen, Bedrohungsfunde, Firewall-Status und Anmeldesicherheit.

## Baseline-Prüfung

- Sicherheits-Baseline-Scans auf dem Host ausführen (z. B. Passwortrichtlinie, SSH-Konfiguration, Systemhärtungspunkte)
- Ergebnisse des letzten Scans und Nichtkonformitäten ansehen
- Scan per Ein-Klick starten

## File-Integrity-Monitoring (FIM)

- Baseline für kritische Dateien / Verzeichnisse erstellen
- Hinzufügen/Löschen/Ändern von Dateien erkennen und Ereignisse aufzeichnen
- Unterstützt Baseline-Neuaufbau und Anzeige von Änderungsereignissen

## Bedrohungserkennung

- Host auf Bedrohungen scannen (anomale Prozesse, verdächtige Dateien usw.)
- Liste der Bedrohungsfunde ansehen
- Bedrohungen als behandelt markieren

## Firewall

- Firewall-Status ansehen
- Firewall aktivieren / deaktivieren
- Ports freigeben / blockieren, Regeln löschen

## Anmeldesicherheit

- Anmeldeverhalten analysieren (Erfolg / Fehlschlag, Quell-IP)
- SSH-Anmeldedatensätze ansehen
- In Kombination mit Begrenzung und Sperrung bei fehlgeschlagenen Anmeldungen

## Zugehörige Berechtigungspunkte

Sicherheitsoperationen werden durch RBAC-Berechtigungspunkte gesteuert: `security.scan`, `security.firewall`, `security.fim`, `security.threat`.
