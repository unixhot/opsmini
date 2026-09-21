# Änderungsprotokoll

Änderungsaufzeichnungen der einzelnen Versionen.

## v1.1.0 (2026-09-21)

**Multi-Host-Verwaltung (Agent / Server-Architektur)**

- **Agent-gRPC-Client**: Der Agent verbindet sich aktiv mit dem OpsMini Server (bidirektionaler gRPC-Stream über einen einzelnen Port), mit Heartbeat und Wiederverbindung per exponentiellem Backoff; er empfängt Ausführungs- / Dateiaufgaben und sendet die Ergebnisse zurück
- **OpsMini-Server-Seite**: Registrierung / Heartbeat / Verbindungsverwaltung, Zustellung von Ausführungsaufgaben, Empfang von Ergebnissen (der Code wurde in das eigenständige Go-Subprojekt `opsmini-server/` des OpsAnt-Projekts verschoben; OpsMini behält nur den Agent)
- **Befehls- / Skriptausführung**: command sowie shell- / python-Skripte; Ergebnisse (stdout / stderr / Exit-Code / Dauer) werden zurückgesendet
- **Datei-Abruf (PullFile)**: Der Server ruft Dateien vom Agent ab (Chunked-Upload + Prüfung von size / md5)
- **Datei-Verteilung (PushFile)**: Der Server verteilt Dateien an den Agent (Chunking + Wiederaufnahme bei Unterbrechung + md5-Prüfung); als Ziel kann ein Verzeichnis angegeben werden (der ursprüngliche Dateiname bleibt erhalten)
- **Erkennung des Verbindungsstatus + Keepalive für langlebige Verbindungen**: Online- / Offline-Status des Agent, Heartbeat-Timeout-Erkennung, gRPC-Keepalive
- **OpsAnt-Anbindung**: Visuelle Konfiguration von OpsAnt-Adresse / Token auf der Seite „Einstellungen“ im Panel (Hot-Switch zur Laufzeit; bei der Token-Anzeige kann per Auge zwischen Klartext / Maskierung umgeschaltet werden), SSO-Sprung ohne Authentifizierung (standardmäßig aktiviert)
- **Entfernung der REST-API `/agent/v1`**: Umstellung auf bidirektionale gRPC-Streams (die bisherige REST-API sowie die Einstellungsseite „API Token“ im Frontend wurden ebenfalls entfernt)

**Erweiterte Panel-Funktionen**

- **Verzeichnis-Download auf der Dateiseite**: Ein Verzeichnis mit einem Klick als zip komprimiert herunterladen (Dialog-Hinweis + Verzeichnisgröße + Festplattenauslastung der betroffenen Partition)
- **Verzeichnis-Löschung auf der Dateiseite**: Rekursives Löschen von Verzeichnissen (Dialog für gefährliche Operationen + zweite Bestätigung durch Eingabe des Verzeichnisnamens)
- **Optimierung langer Namen auf der Dateiseite**: Überlange Datei- / Verzeichnisnamen werden gekürzt angezeigt, der vollständige Name erscheint beim Hover (keine horizontale Scrollleiste mehr)
- **Routing-Tabelle unter „Netzwerk“ in der Systemverwaltung**: Neu hinzugefügte Routing-Tabelle unterhalb der Netzwerkschnittstelle (Zielnetzwerk / Gateway / Subnetzmaske / Flags / Interface / Metrik)
- **Erweiterte MCP-Konfiguration**: Unterstützung zweier Eingabemethoden, „Konfigurationsdatei (JSON)“ und „Tabelle“; bei SSE / HTTP werden benutzerdefinierte Header (mehrere) und eine Timeout-Zeit unterstützt
- **Erweiterte Firewall**: Vor der Aktivierung werden Panel-Port und SSH-Port (22) automatisch freigegeben, um nach der Aktivierung keinen Zugriffsverlust zu riskieren; ufw im Zustand „installiert, aber nicht aktiviert“ wird erkannt
- **Erweiterte Prometheus-Metriken**: Neu hinzugefügt: `opsmini_cpu_usage_percent` / `opsmini_mem_usage_percent` für die aktuelle Auslastung sowie `node_procs_total` für die Gesamtzahl der Prozesse
- **Einheitlicher Bestätigungsdialog**: Alle Lösch- / gefährlichen Operationen verwenden nun den integrierten Bestätigungsdialog der Plattform (ersetzt das native confirm des Browsers)

**Terminal-Optimierungen**

- **Ausgabeabstände**: Die Befehlsausgabe erhält zusätzlichen Rand links / rechts und unten und liegt nicht mehr direkt am Rand an
- **Esc-Fix im Vollbildmodus**: Im Vollbildmodus beendet Esc den Vollbildmodus nicht mehr (vermeidet Konflikte mit Esc im vim-Editiermodus)
- **Sitzungserhaltung**: Nach dem Seitenwechsel wird die Terminal-Sitzung nicht mehr zurückgesetzt (sie bleibt während der Gültigkeitsdauer der Sitzung bestehen)
- **Zeichensatz-Einstellung**: Neu hinzugefügte Auswahl des Zeichensatzes (UTF-8 / GBK / GB18030 / Big5), um die fehlerhafte Darstellung chinesischer Zeichen zu beheben

**Optimierungen der Benutzererfahrung**

- **Seitentitel**: Zeigt „OpsMini · Zugriffsadresse“ an, sodass mehrere Tabs unterscheidbar sind
- **Passwort-Sichtbarkeit**: Eingabefelder wie das Ändern des Passworts oder der OpsAnt-Anbindungs-Token erhalten ein neues Auge-Symbol für „Anzeigen / Ausblenden“
- **Layout der Einstellungsseite**: „Zwei-Faktor-Authentifizierung“ und „OpsAnt-Anbindung“ werden nebeneinander dargestellt; der Schalter „Automatische Updates“ wurde entfernt

## v1.0.0 (2026-09-13)

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
