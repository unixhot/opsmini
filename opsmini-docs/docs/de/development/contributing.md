# Beitragsleitfaden

Vielen Dank für dein Interesse an OpsMini und deine Beiträge!

## Mitwirkungsmöglichkeiten

- **Bug melden**: in GitHub Issues einreichen, mit Reproduktionsschritten, Umgebungsinformationen und Logs
- **Funktionsvorschläge**: in Issues Anwendungsszenario und Erwartung beschreiben
- **Code einreichen**: Repository forken → neuen Branch erstellen → committen → Pull Request eröffnen
- **Dokumentation verbessern**: Dokumentationsfehler korrigieren, Verwendungsbeispiele ergänzen

## Commit-Konventionen

- Ein PR fokussiert sich auf eine Änderung, vermeide „groß und allumfassend"
- Dem offiziellen Go-Codestil folgen (`gofmt` / `go vet`)
- Für neue Funktionen Testfälle ergänzen
- Commit-Nachrichten beschreiben klar „was getan wurde" und „warum"

## Verzeichnisvereinbarungen

- Schichtung: `api/v1` (Controller) → `service` (Geschäftslogik) → `repository` (Daten)
- Sensible Felder (Schlüssel / Token) verschlüsselt speichern
- Neue Schnittstellen folgen der einheitlichen Antwort `{ code, message, data }`

## Lizenz

Das Projekt verwendet die Apache License 2.0; mit dem Einreichen von Code gilt die Zustimmung zur Lizenzierung unter dieser Lizenz als erteilt.
