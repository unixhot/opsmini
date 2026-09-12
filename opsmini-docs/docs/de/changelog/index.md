# Änderungsprotokoll

Änderungsaufzeichnungen der einzelnen Versionen.

## v1.0.0 (aktuell · erste offizielle Veröffentlichung)

Die erste offizielle Version von OpsMini, ausgeliefert als einzelnes Binary, mit diesen Kernfähigkeiten:

- **AI-Betrieb**: Diagnose in natürlicher Sprache, intelligente Log-Analyse, Befehlsgenerierung und -ausführung (SSE-Streaming-Ausgabe), MCP-Tool-Anbindung
- **Monitoring & Alarme**: Zeitreihenerfassung für CPU / Speicher / Festplatte / Netzwerk; Alarmregeln unterstützen Benachrichtigungen per E-Mail, Panel, WeCom-, DingTalk- und Feishu-Gruppen-Bot
- **Host-Sicherheit**: Sicherheitsbewertung, Baseline-Prüfung, File-Integrity-Monitoring (FIM), Bedrohungserkennung, Firewall, Anmeldesicherheit
- **App-Store**: Anwendungskategorien + Ein-Klick-Installation + Synchronisierung offizieller Vorlagen
- **AI-Skills (Skill Hub)**: Skill-Suche, Installation, Upload
- **Web-Terminal**: interaktive Shell per WebSocket + pty, mehrere Sitzungen
- **Regulärer Betrieb**: Websites, Datenbanken, geplante Aufgaben, Dateiverwaltung
- **Zwei-Faktor-Authentifizierung (2FA)**: globaler Schalter + TOTP-Bindung pro Benutzer
- **Monitoring-Export**: Prometheus-`/metrics`-Endpunkt (node_exporter-kompatibel)
- **Benachrichtigungsverwaltung**: SMTP / WeCom / DingTalk / Feishu
- **Verschlankte Konfiguration**: JWT-Schlüssel, Agent-Token und AI-Konfiguration sind vollständig im Panel verankert; die Konfigurationsdatei enthält nur noch die minimalen Elemente
- **Log-System**: konfigurierbares Log-Level (info / warn / error), Logs werden auf die Festplatte geschrieben
- **Konto-Wiederherstellung**: Unterbefehle `-reset-pass` / `-reset-mfa`
- **i18n in 7 Sprachen**: vereinfachtes / traditionelles Chinesisch, Englisch, Japanisch, Koreanisch, Thailändisch, Deutsch
- **Weiße minimalistische UI**: weißes Farbschema mit schwarzer Schrift, akzentuiert durch Markenblau
- **Doppelte API**: `/api/v1` (für den Browser) + `/agent/v1` (standardmäßige REST zwischen Maschinen)

> Die vollständige Versionshistorie richtet sich nach den Releases im Repository: [GitHub Releases](https://github.com/unixhot/opsmini/releases)
