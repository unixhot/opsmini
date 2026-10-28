# Änderungsprotokoll

Änderungsaufzeichnungen der einzelnen Versionen.

## v1.3.0 (2026-10-28)

**Autonomer Agent: Von „Du fragst, er antwortet" zu „Er behält es selbst im Blick"**

Diese Version bringt OpsMini von einem „KI-Assistenten, der Anweisungen ausführt" zu einem permanenten Agenten, der „selbst Probleme erkennt, selbst urteilt und innerhalb deiner Grenzen handelt". Der Kern ist ein geschlossener Kreislauf von der Erfassung bis zur Verifikation – nicht nur ein paar weitere Buttons.

**Wahrnehmungsschicht (internal/sentinel)**

- Sechs Collector-Typen im Dauerbetrieb: Log-Explosion (stündliche EWMA-Baseline + harte Schwelle von 20MB/min), Anmelde-Anomalien (außerhalb der Arbeitszeit / erstmalig auftretende IP / Erfolg nach Brute-Force / `authorized_keys` gerade geändert), Skript-Ablage in Web-Verzeichnissen (14 WebShell-Merkmale, starke Merkmale direkt high), Prozess-Spawning (Web-Benutzer startet shell / curl / python), verdächtige ausgehende Verbindungen, Metrik-Alarme
- Erfassungsergebnisse werden einheitlich zu **Signal** normalisiert: Fingerprint-Deduplizierung, Persistenz in SQLite; Übergabe über prozessinternen channel, keine Message-Queue
- Erfassungsfrequenz gestaffelt 2s–20s, bei anomalen Logs wird der Prozess ermittelt, der die Datei hält, um „Log wächst, aber niemand weiß, wer schreibt" zu vermeiden

**Event-Center**

- Signale werden nach Fingerprint und Zeitfenster zu **Events** aggregiert; ein Event trägt Titel, Kategorie, Stufe, Objekt, Häufigkeit
- Sechs Filter (ausstehend / aufzuwecken / alle / erledigt / fehlgeschlagen / ignoriert) mit Zählern; bei Auswahl wird die Definition des jeweiligen Status angezeigt
- Event-Details lassen sich bis zu zugehörigen Signalen, Agent-Ausführungsprotokollen und der einzelnen Aktionskette (Tool, Parameter, Entscheidung, Rohausgabe) aufschlüsseln

**Policy-Engine (internal/trigger)**

- Fünf integrierte Policies sofort einsatzbereit: `webshell-respond`, `auth-anomaly`, `proc-anomaly` (nur critical), `log-growth` (Shadow-Modus), `net-anomaly` (standardmäßig deaktiviert)
- Drei Bedingungen müssen gleichzeitig erfüllt sein, um den Agenten zu wecken: Kategorie passt, Stufe erreicht Schwelle, nicht in Abklingzeit; andernfalls wird nur das Event protokolliert, **ohne Modellkontingent zu verbrauchen**
- Jede Policy kann einzeln Budgets konfigurieren: Anzahl Schreiboperationen, Modell-Schritte, Wall-Clock-Dauer, Token-Obergrenze; bei Überschreitung eines Werts wird gestoppt
- Policies sind Daten, kein Code: im UI hinzufügen/ändern/löschen, Schwellen anpassen, Modus wechseln, ohne Neustart

**Drei Betriebsmodi**

- `shadow` Nur beobachten: wie gewohnt Beweise sichern, Schlussfolgerungen und „was ich vorhätte" schreiben, keine einzige Schreiboperation ausführen – geeignet, um erst einige Tage zur Kalibrierung der Beurteilung zu laufen
- `assist` Unterstützen: Leseaktionen werden automatisch ausgeführt, jede Schreibaktion hält an und wartet auf Bestätigung – empfohlene Stufe für den Alltag
- `auto` Autonom: nur risikoarme Aktionen aus der `auto_tools`-Whitelist sind bestätigungsfrei, ab L2 ist immer ein menschliches OK nötig

**Irreversible Aktionen reversibel machen**

- Löschen → **Isolieren**: Datei wird in den Isolationsbereich verschoben, Originalpfad, Prüfsumme, Berechtigungen werden erfasst, Ein-Klick-Wiederherstellung
- `kill -9` → **SIGSTOP-Einfrieren**: erst anhalten und den Zustand bewahren, nach Bestätigung entscheiden
- Dauerhafte Sperre → **temporäre Sperre mit TTL**: automatische Aufhebung bei Ablauf, Fehlsperren werden kein Vorfall
- Vor dem Handeln erst **Snapshot**: vor Konfigurationsänderungen ein Backup, bei fehlgeschlagener Verifikation Rollback möglich

**Fünf Sicherheitsschranken (internal/safety)**

- Parameter-Policy, Schreibbudget, geschützte Pfade, Not-Aus-Schalter (killswitch), Assertion-Verifikation nach der Behandlung (`no_new_signal`, `port_listen`, `file_absent` usw.)
- Events mit fehlgeschlagener Verifikation werden als fehlgeschlagen markiert und an den Menschen zurückgegeben, statt sie als Erfolg zu verschleiern; leere Berichte gelten ebenfalls als fehlgeschlagen
- Jeder Tool-Aufruf geht in die Aktionsprüfung: wer ihn ausgelöst hat, was aufgerufen wurde, welche Parameter übergeben wurden, was zurückkam

**Datenaufbewahrung**

- Panel-Einstellungen erhalten eine neue Seite „Datenaufbewahrung": Events und Signale standardmäßig 30 Tage, Aktionsprüfung 180 Tage, Isolationsobjekte und Sperrprotokolle 30 Tage
- Ausstehende Events samt zugehörigen Signalen und Aktionsketten werden **niemals automatisch gelöscht**; isolierte Dateien bleiben bis zur Wiederherstellung unangetastet, Sperren bis zur Aufhebung
- Zusätzlich „Event-Verlauf leeren": ignoriert das Aufbewahrungsfenster und leert einmalig alles (inkl. isolierter Dateikopien und Sperrprotokolle, Entfernen von Firewall-Regeln, Zurücksetzen der Policy-Abklingzeiten), Ausführung nach doppelter Bestätigung

**Drill und Verifikation**

- Neu: `scripts/chaos-drill.sh`: Injektion von vier Fehlertypen (Log-Explosion / WebShell-Ablage / erfolgreicher SSH-Brute-Force / Web-Benutzer spawnt shell), anschließend API-Prüfung, ob die Wahrnehmungsschicht die entsprechenden Events erzeugt
- Auf einer öffentlichen Testmaschine alle vier Szenarien bestanden; im `auto`-Modus echte Isolierung einer WebShell und Verifikation bestanden

**Weitere Verbesserungen**

- Login-Seiten-Text auf Agent-Positionierung umgestellt (7 Sprachen), Tab-Titel folgt dem Anmeldestatus
- Benachrichtigungskarten lassen sich durch Klick auf leere Seitenbereiche schließen
- Build-Versionsstempel auf `git describe --tags --dirty` umgestellt, neu `make release-check` Release-Vorprüfung, um Releases mit schmutzigem Arbeitsbereich zu vermeiden

**Upgrade-Hinweise**

- v1.2.1 → v1.3.0: einfach das Binary ersetzen und neu starten, neue Tabellen werden automatisch angelegt, keine Migration nötig
- Nach dem Upgrade empfiehlt es sich, im „Event-Center → Policies" einen Blick auf die fünf integrierten Policies zu werfen: standardmäßig sind alle `assist` oder `shadow`, das System wird nicht automatisch verändert

## v1.2.1 (2026-10-27)

**Korrektur: vier Mehrhost-KI-Werkzeuge entfernt, die eine Grenze überschritten**

v1.2.0 registrierte vier Mehrhost-Werkzeuge (`list_remote_hosts` / `remote_exec` / `remote_pull_file` / `remote_push_file`), damit die KI des Panels über die HTTP-API der Steuerebene des OpsMini Server (4508) Befehle und Dateien an andere erfasste Hosts schickt. Diese Umsetzung überschritt eine Architekturgrenze und wurde entfernt.

- 4508 ist eine **interne** API, die der OpsMini Server OpsAnt bereitstellt, gedacht für „nur lokal / internes Netz“, und sie prüft keine Anmeldedaten.
- Auf dem Mehrhost-Weg ist das Panel der **verwaltete Agent** und sollte ausschließlich über die gRPC-Langzeitverbindung auf 4507 mit dem Server sprechen (Registrierung, Heartbeat, Auftragsannahme) — nicht die Steuerebene zurückrufen.
- Damit dieses Design funktioniert, müsste 4508 nach außen geöffnet werden — also ein unauthentifizierter Einstiegspunkt zur entfernten Befehlsausführung im Internet.

Damit sind es wieder **29 Werkzeuge** (16 lesend / 5 risikoarm schreibend / 8 risikoreich schreibend). Mehrhost-Orchestrierung und Mehrhost-KI übernimmt OpsAnt mit seiner einheitlichen Ausführungs-API (`/api/opsant/v1/tasks/run-shell/`, `tasks/run-script/`, `tasks/send-file/`, `tasks/get-minion-file/` usw.) und drei Ausführungsengines.

**Keine Migration nötig**: Die vier Werkzeuge setzten auch in der alten Version voraus, dass 4508 erreichbar ist, und waren praktisch nie nutzbar.

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
