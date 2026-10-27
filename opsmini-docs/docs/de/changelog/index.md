# Änderungsprotokoll

Änderungsaufzeichnungen der einzelnen Versionen.

## v1.2.0 (2026-10-27)

**KI-Agent: vom Betriebsberater zum Betriebsausführenden**

- **Eigener schlanker ReAct-Kern**: Reason → Act → Observe ohne Agent-Framework, damit das einzelne Binary und der Build ohne CGO erhalten bleiben. Ein Durchlauf ist auf 12 Schritte begrenzt; danach stoppt der Agent die Werkzeugaufrufe und berichtet den bekannten Stand
- **33 eingebaute Werkzeuge**: 16 lesende (Systemübersicht, Prozesse, Ports, Datenträger, Verzeichnisgröße, Logsuche, Dienste, Container, Sicherheit u. a.), 5 risikoarme Schreibvorgänge, 8 risikoreiche Schreibvorgänge sowie 4 Mehrhost-Werkzeuge, die dynamisch aus der Agent-Konfiguration registriert werden
- **Gestreamte Werkzeugaufrufe**: `tool_calls`-Fragmente werden je Index gesammelt und zusammengesetzt; Anbieter ohne Streaming-Unterstützung fallen automatisch auf einen nicht gestreamten Versuch zurück
- **Sichtbare Denkkette**: Die Oberfläche zeigt schrittweise, welches Werkzeug mit welchen Argumenten lief und welche Rohausgabe zurückkam, sodass das Ergebnis prüfbar ist

**Abgestufte Berechtigungen und menschliche Freigabe**

- **Vierstufiges Risikomodell**: L0 nur lesend läuft frei / L1 risikoarm fragt standardmäßig nach (pro Werkzeug freistellbar) / L2 risikoreich fragt immer nach und ist nie freistellbar / L3 katastrophale Vorgänge werden per Muster hart blockiert
- **L3 blockiert vor der Ausführung**: `rm -rf /`, `mkfs`, `dd of=/dev/sd*`, Fork-Bomben, das Leeren der Firewall, das Überschreiben von `/etc/passwd` und Ähnliches werden abgelehnt, selbst wenn ein Administrator zustimmt. Zwei weitere Regeln schützen das Panel selbst (Datenverzeichnis nicht anfassen, Panel-Port nicht schließen)
- **Unterbrechen und Fortsetzen**: Ein risikoreicher Aufruf wird gespeichert und angehalten; nach Zustimmung oder Ablehnung im Chat wird die Nachrichtenfolge aus SQLite fortgesetzt. Der Ablehnungspfad wurde nachweislich ohne Nebenwirkungen geprüft

**Bidirektionales MCP**

- **MCP-Client**: auf Basis des offiziellen MCP-Go-SDK mit stdio und Streamable HTTP. Werkzeuge externer MCP-Server werden als `mcp__<Server>__<Werkzeug>` eingebunden; die Konfiguration wird alle 2 Minuten abgeglichen
- **MCP-Server**: veröffentlicht das eigene Werkzeugset unter `/mcp` hinter einem Bearer-Token und gibt standardmäßig nur L0/L1 frei (L2 wird nicht an nicht autorisierte Clients ausgeliefert, da diese den Bestätigungsdialog des Panels nicht darstellen können)

**Geplante KI-Aufgaben**

- **Vierter Aufgabentyp „Agent-Aufgabe"**: Prompt schreiben und nach Cron ausführen. Zeitplan, Aktivierung, letzte Ausführung und Fehlermeldungen nutzen die vorhandene Mechanik vollständig mit
- **Berechtigungen werden unbeaufsichtigt herabgestuft**: Beim planmäßigen Lauf klickt niemand auf „Zustimmen", daher wird jedes L2-Werkzeug direkt abgelehnt und der Grund an das Modell zurückgegeben. Das Modell wechselt auf lesende Werkzeuge, führt die Aufgabe zu Ende und vermerkt im Bericht, dass der Vorgang einen Menschen erfordert, samt kopierbarem Befehl
- **Zuverlässigkeit und Kosten**: Schrittlimit und Gesamt-Timeout pro Aufgabe; drei aufeinanderfolgende Fehlschläge deaktivieren die Aufgabe automatisch, damit ein defekter Job nicht ewig alarmiert. Speichern im Panel setzt den Fehlerzähler zurück
- **Fünf eingebaute Prüfvorlagen**: Host-Prüfung, Sicherheitsprüfung, Speicherplatz, Dienststatus, Log-Auffälligkeiten, dazu vier gängige Zeitplanvorlagen

**Einzelne HTML-Berichtsdatei**

- **Ein Bericht ist eine eigenständige HTML-Datei**: Inline-Styles, kein Skript, kein externes Stylesheet, keine Web-Schrift, keine Bilder – sie lässt sich vom USB-Stick, als Mailanhang oder von jedem statischen Server aus anzeigen
- **Öffnen oder herunterladen**: Jede Aufgabenzeile bietet „Öffnen" (direkt in einem neuen Tab) und „Herunterladen"; chinesische Dateinamen werden nach RFC 5987 kodiert
- **Archiviert im Datenverzeichnis**: `.html` und `.md` landen unter `<Datenverzeichnis>/reports/<Aufgabenname>/`; alle erzeugten Artefakte liegen je Funktion in einem Unterverzeichnis
- **Sichere Darstellung**: Der Berichtstext mischt Modellprosa mit Host-Daten (Logauszüge, Prozess-Kommandozeilen, Konfigurationswerte). Der Renderer maskiert zuerst das gesamte Dokument als HTML und wendet erst danach Inline-Regeln an; nur http/https/mailto-Ziele sind erlaubt, sodass ein `<script>` in einer Logzeile als Text erscheint. Zusätzlich wird eine strikte CSP gesendet
- **Wenn PDF benötigt wird**: Im geöffneten Bericht „Drucken → Als PDF speichern" verwenden; das Stylesheet enthält bereits Druckregeln

**Benachrichtigungen und Audit**

- **Gemeinsamer Notifier**: Der Benachrichtigungsversand wurde aus den Alarmen in einen gemeinsamen `Notifier` extrahiert (Panel / E-Mail / WeCom / DingTalk / Feishu), sodass KI-Aufgaben und Alarme identisch reagieren. Ohne Kanalauswahl wird die Meldung im Panel zugestellt
- **Audit-Spur**: KI-Werkzeugaufrufe landen im Audit-Log; Ablehnungen im unbeaufsichtigten Lauf als `unattended-deny`, menschliche Ablehnungen mit dem Präfix `reject:`

**Weitere Verbesserungen**

- Die Versionsangabe in der Seitenleiste zeigt nun die echte Backend-Version statt eines festen Werts
- Das Design-Dokument `docs/ai-agent.md` wurde in 7 Sprachen ergänzt
- Administratoren können die Werkzeugrichtlinien unter „KI-Assistent → Werkzeugrechte" anpassen; risikoreiche Werkzeuge bleiben immer auf „Nachfragen" gesperrt

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
