<div align="center">

# OpsMini AI-Agent-Design (AI Agent)

**AI Agent Design**

[English](ai-agent.md) · [简体中文](ai-agent.zh-CN.md) · [繁體中文](ai-agent.zh-TW.md) · [日本語](ai-agent.ja.md) · [한국어](ai-agent.ko.md) · [ไทย](ai-agent.th.md) · [Deutsch](ai-agent.de.md)

</div>

---

> Meilensteine M8–M10 ｜ Code: `internal/aiagent/`
>
> Verwendung: AI Assistant → Modell konfigurieren → direkt in natürlicher Sprache beauftragen

In M8 wurde der AI-Assistent von OpsMini von einem „Betriebsberater, der einen Live-Systemauszug lesen kann" zu **einem Agenten, der den Host tatsächlich bedienen kann**: Das Modell plant, ruft Werkzeuge auf, beobachtet die Ergebnisse und iteriert, bis es zu einem Schluss kommt — während jede risikoreiche Aktion weiterhin eine ausdrückliche menschliche Freigabe erfordert.

## 1. Warum ein eigener, schlanker Kernel statt eines Agent-Frameworks

Für Go gibt es ausgereifte Agent-Frameworks (Google ADK Go, CloudWeGo Eino). Trotzdem baut OpsMini einen eigenen Kernel von rund 1.500 Zeilen, weil:

1. **Der Burggraben liegt in der Fachdomäne, nicht in der Orchestrierung.** Frameworks lösen die generische Schleife aus Modell und Werkzeugen; der Wert von OpsMini liegt in seinen 29 Domänenwerkzeugen, den abgestuften Schutzleitplanken und dem gRPC-Kanal für mehrere Hosts — nichts davon liefert ein Framework.
2. **Der Klebstoff kostet etwa so viel wie der Kernel.** Die Werkzeuge sind gewöhnliche lokale Go-Funktionen; sie an die Werkzeugschnittstelle eines Frameworks anzupassen, ist vergleichbar viel Arbeit.
3. **Ein einziges Binary, kein CGO.** ADK zieht OpenTelemetry, genai und GCS in den Abhängigkeitsbaum.
4. **Ein Protokoll als Erweiterungspunkt, kein Framework.** Werkzeuge werden mit **MCP-kompatiblem JSON Schema** deklariert, niemals in einem framework-eigenen Format. Als Nebeneffekt lässt sich dieselbe Registrierung direkt als MCP-Server veröffentlichen.

Die Entscheidung lautet daher: **protokollbasierte Werkzeuge + eigener Kernel.** Sollten je Multi-Agent-Zusammenarbeit oder visuelle Workflow-Orchestrierung nötig werden, bleibt der Wechsel zu Eino ADK oder Google ADK Go möglich, während OpsMini über MCP / A2A interoperiert.

## 2. Aufbau des Kernels

```
internal/aiagent/
├── types.go      Tool / Result / Message / ToolCall / RiskLevel
├── registry.go   Threadsichere Registrierung (OpenAI- und MCP-Definitionen)
├── llm.go        OpenAI-kompatibler Client (Akkumulation von Streaming-Tool-Calls)
├── loop.go       ReAct-Kernel: Denken → Handeln → Beobachten, mit Unterbrechen/Fortsetzen
├── guard.go      L3-Hartverbote, Selbstschutz des Panels, Risiko→Richtlinie
├── hitl.go       Persistierte, ablaufende menschliche Freigaben
├── memory.go     Gesprächspersistenz und Kontextkompression
├── mcpclient.go  MCP-Client (offizielles Go SDK, stdio + Streamable HTTP)
├── mcpserver.go  MCP-Server (veröffentlicht das Werkzeugset an externe Clients)
├── events.go     SSE-Ereignismodell
└── tools/        Das eingebaute Werkzeugset
```

## 3. Die ReAct-Schleife

Ein Nutzerzug entspricht einem `Agent.Send`:

1. Das Gesprächsprotokoll wird aus SQLite geladen, davor System-Prompt und ein Live-Auszug des Hosts gesetzt.
2. Das Modell wird **streamend** aufgerufen; Textdeltas gehen als `thinking`-Ereignisse an das Frontend.
3. Wenn das Modell Werkzeuge anfordert:
   - zuerst läuft der **L3-Hartverbotsscan** — ein Treffer wird abgelehnt und zurückgemeldet;
   - danach entscheiden Risikostufe und die vom Administrator gesetzte Richtlinie je Werkzeug: sofort ausführen / Freigabe verlangen / deaktiviert;
   - die Beobachtung geht als `tool`-Nachricht zurück, die Schleife läuft weiter.
4. Fordert das Modell keine Werkzeuge mehr an, wird die endgültige Antwort ausgegeben.

Das Schrittbudget beträgt standardmäßig 12 pro Zug; darüber hinaus ruft der Agent keine Werkzeuge mehr auf und berichtet, was er bereits weiß — ein Lauf kann also nicht davonlaufen.

## 4. Risikostufen und Schutzleitplanken

| Stufe | Name | Verhalten |
|-------|------|-----------|
| L0 | `RiskReadonly` | Nur lesend, läuft unbeaufsichtigt |
| L1 | `RiskLowWrite` | Schreibzugriff mit geringem Risiko; **standardmäßig dennoch bestätigungspflichtig**, ein Administrator kann einzelne Werkzeuge für unbeaufsichtigte Ausführung freigeben |
| L2 | `RiskHighWrite` | Hohes Risiko; **jeder Lauf braucht menschliche Freigabe** und kann nie auf automatisch gestellt werden |
| L3 | Musterbasiert | Wird vor der Ausführung blockiert, selbst mit Freigabe |

L3 umfasst `rm -rf /` (einschließlich kritischer Systemverzeichnisse), `mkfs`, `dd of=/dev/sd*`, Umleitungen auf Blockgeräte, Herunterfahren/Neustart, `chmod -R 777 /`, `chown -R … /`, Fork-Bomben, das Leeren von iptables-/nft-Regeln, das Überschreiben von Authentifizierungsdateien wie `/etc/passwd`, das Löschen der Shell-Historie, `rmmod` und `kill -9 1`.

Zwei weitere Leitplanken schützen das Panel selbst:

- Alles, was das Datenverzeichnis des Panels löschen oder überschreiben würde (Binary, SQLite-Datenbank, Skills), wird abgelehnt.
- Firewall-Regeln, die den lauschenden Port des Panels abschneiden würden, werden abgelehnt.

> Produktentscheidung: kein unbeaufsichtigter Root-Agent. Zielbild ist **„autonome Planung + abgestufte Autorisierung"** — die KI untersucht und berät, der Mensch autorisiert alles Destruktive.

## 5. Human-in-the-loop

Braucht ein Aufruf eine Freigabe, schreibt der Kernel einen Datensatz in `ai_pending_approvals` (Sitzung, Werkzeug, Argumente, Risiko, Vorschautext, Ablauf — standardmäßig 10 Minuten), sendet ein `confirm`-SSE-Ereignis und **beendet den Zug**; das Frontend zeigt eine Bestätigungskarte.

Nach dem Freigeben oder Ablehnen stellt `POST /ai/agent/approve` die vollständige Nachrichtenfolge aus der Datenbank wieder her und setzt fort:

- Werkzeugergebnisse werden immer in Aufrufreihenfolge persistiert, und der Lauf stoppt beim ersten unbestätigten Aufruf. Daher ist **der Index des unterbrochenen Aufrufs gleich der Anzahl der Werkzeug-Nachrichten, die auf diese Assistant-Nachricht folgen** — das Fortsetzen braucht keine zusätzliche Buchführung.
- Aufrufe, die im selben Zug dahinter anstanden, erhalten ein ausdrückliches „nicht ausgeführt", damit das Protokoll für das Modell gültig bleibt.

## 6. Werkzeugbestand (29)

| Klasse | Anzahl | Beispiele |
|--------|--------|-----------|
| L0 nur lesend | 16 | `sys_overview`, `list_processes`, `list_ports`, `dir_usage`, `read_file`, `search_logs`, `query_metrics`, `docker_ps`, `security_status` |
| L1 risikoarm schreibend | 5 | `clean_temp`, `clean_logs`, `service_control`, `docker_control`, `install_skill` |
| L2 risikoreich schreibend | 8 | `exec_command`, `write_file`, `delete_path`, `modify_sshd`, `compress_dir`, `cron_write`, `kill_process`, `firewall_rule` |
| Mehrere Hosts (optional) | 4 | `list_remote_hosts`, `remote_exec`, `remote_pull_file`, `remote_push_file` |
| Externes MCP | dynamisch | `mcp__<server>__<tool>`, Risiko aus der `readOnlyHint`-Annotation abgeleitet |

Werkzeuge erben das RBAC des Panels: Eine nur lesende Rolle sieht keine Werkzeuge, die Schreibrechte benötigen, und L2-Werkzeuge verlangen zusätzlich die Berechtigung `ai.agent.write`.

## 7. MCP in beide Richtungen

### Als MCP-Client (externe Werkzeuge nutzen)

MCP-Server werden unter „AI Assistant → MCP" konfiguriert (stdio-Befehl oder Streamable-HTTP-URL mit eigenen Headern). Der Kernel verbindet sich mit dem offiziellen MCP-Go-SDK, listet die Werkzeuge auf und registriert jedes als `mcp__<server>__<tool>`. Verbindungen werden alle zwei Minuten abgeglichen, Konfigurationsänderungen brauchen also keinen Neustart.

### Als MCP-Server (Fähigkeiten veröffentlichen)

Das Panel veröffentlicht sein eigenes Werkzeugset unter `/mcp` (Streamable HTTP):

- Die Authentifizierung nutzt ein **Bearer-Token**, das beim ersten Start erzeugt wird und unter „AI Assistant → Werkzeugberechtigungen" einsehbar, kopierbar und erneuerbar ist. MCP-Clients können die JWT-Anmeldung des Panels nicht durchführen, daher das eigene Token.
- Standardmäßig werden nur **L0-/L1-Werkzeuge** veröffentlicht. L2-Werkzeuge bleiben verborgen, bis „Riskante MCP-Aktionen erlauben" aktiviert wird, denn ein externer MCP-Client kann den Bestätigungsdialog des Panels nicht anzeigen.
- Die L3-Leitplanken und der Selbstschutz des Panels gelten auch für entfernte Aufrufe.
- Jeder entfernte Aufruf landet mit dem Akteur `mcp-client` im Audit-Protokoll.

Beispiel für die Client-Konfiguration (Claude Desktop / Cursor und andere Streamable-HTTP-Clients):

```json
{
  "mcpServers": {
    "opsmini": {
      "type": "http",
      "url": "https://your-panel.example.com/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

## 8. Sitzungen und Gedächtnis

- `ai_sessions` / `ai_messages` liegen in SQLite und zeichnen `tool_calls` und `tool_call_id` auf; ein Neuladen der Seite oder ein Prozessneustart kann die Gedankenkette fortsetzen oder wiedergeben.
- Wächst das Protokoll über eine Schwelle (24 Nachrichten wörtlich, plus 12 Nachrichten Puffer), wird es **komprimiert**: ältere Runden werden vom Modell zu einer Zusammenfassung von höchstens 300 Zeichen verdichtet und als eine System-Nachricht wieder eingefügt.
- Sitzungen werden standardmäßig 30 Tage aufbewahrt (`ai_session_retention_days`) und im Hintergrund bereinigt.

## 9. API-Übersicht

| Methode | Pfad | Zweck |
|---------|------|-------|
| POST | `/api/v1/ai/agent/chat` | Streamender Chat (Gedankenkette / Werkzeugaufrufe / Bestätigungen) |
| POST | `/api/v1/ai/agent/approve` | Einen ausgesetzten Aufruf freigeben oder ablehnen und fortsetzen |
| GET | `/api/v1/ai/agent/tools` | Werkzeugbestand (Risiko, Berechtigung, Richtlinie) und MCP-Status |
| PUT | `/api/v1/ai/agent/tools/policy` | Richtlinie je Werkzeug (automatisch / bestätigen / deaktiviert) |
| GET | `/api/v1/ai/agent/sessions` | Sitzungsliste |
| GET | `/api/v1/ai/agent/sessions/:id/messages` | Protokoll (einschließlich offener Freigaben) |
| DELETE | `/api/v1/ai/agent/sessions/:id` | Sitzung löschen |
| GET | `/api/v1/ai/agent/config` | Laufzeitkonfiguration und Modellzustand |
| GET | `/api/v1/ai/agent/sessions/:id/report` | Bericht einer Sitzung exportieren (standardmäßig HTML) |
| GET | `/api/v1/ai/agent/reports` | Archivierte Berichte auflisten (mit Format und Archivverzeichnis) |
| GET | `/api/v1/ai/agent/reports/view?name=` | Archivierten Bericht **direkt öffnen** (inline, akzeptiert auch `?token=`) |
| GET | `/api/v1/ai/agent/reports/download?name=` | Archivierten Bericht herunterladen (attachment, akzeptiert auch `?token=`) |
| GET/PUT | `/api/v1/ai/agent/mcp-server` | Konfiguration des eingehenden MCP-Endpunkts (aktiviert / Token / hohes Risiko) |
| ANY | `/mcp` | Eingehender MCP-Endpunkt (Bearer-Token) |

Berechtigungen: `ai.agent` (Agent nutzen) und `ai.agent.write` (risikoreiche Aktionen autorisieren). Die älteren Endpunkte `/ai/chat` und `/ai/chat/stream` bleiben als Ausweichkanal erhalten.

## 10. Verwaltung mehrerer Hosts

Sobald das Panel ein `agent.server_addr` hat (die Datenebene des OpsMini Server auf dem OpsAnt-Host), werden vier Multi-Host-Werkzeuge automatisch registriert. Sie nutzen die HTTP-API der Steuerebene des Servers (standardmäßig Port 4508 auf demselben Host), um auf anderen erfassten Hosts Befehle oder Skripte auszuführen und Dateien zu übertragen — ganz ohne SSH. `ai_remote_server_url` / `ai_remote_server_token` überschreiben die abgeleitete Adresse.

## 11. Geplante KI-Aufgaben (Host-Prüfung und Verwandtes)

Eine „KI-Aufgabe" ist kein neuer Einstiegspunkt, sondern die **vierte Art geplanter Aufgabe** (`kind = agent`), neben „Panel-Aufgaben / System-crontab / Benutzer-crontab". Zeitplanung, Aktivieren/Deaktivieren, letzte Ausführung und Fehleralarme werden also vollständig wiederverwendet, und „morgens etwas ausführen" liegt dort, wo Nutzer ohnehin nachsehen.

### Ablauf

```
cron feuert -> Agent läuft unbeaufsichtigt (ReAct mit nur lesenden / risikoarmen Werkzeugen)
            -> Protokoll wird gespeichert (der Bericht IST eine KI-Sitzung, samt Gedankenkette)
            -> HTML und Markdown werden unter <Datenverzeichnis>/reports/<Aufgabenname>/ archiviert
            -> Zusammenfassung geht an die Benachrichtigungskanäle der Aufgabe
```

### Bei unbeaufsichtigten Läufen wird die Bestätigungsrichtlinie zu „ablehnen" abgestuft

Niemand ist da, um „Freigeben" zu klicken. Mit `RunOptions.Unattended = true` gilt daher:

- Ein Werkzeug, das auf `PolicyConfirm` (L2) abbildet, wird **sofort abgelehnt**; dem Modell wird der Grund genannt („nutze stattdessen ein nur lesendes Werkzeug und halte im Fazit fest, dass dieser Schritt einen Menschen braucht") und die Ablehnung landet in der `Skipped`-Liste der Sitzung.
- Das Modell wechselt auf nur lesende Werkzeuge und die Aufgabe läuft trotzdem zu Ende; der Bericht benennt die Aktion, die einen Menschen braucht, und den exakten auszuführenden Befehl.
- L0- und L1-Werkzeuge sind nicht betroffen. Eine tägliche Prüfung ist damit konstruktiv nur lesend; eine Ausweitung erfordert eine ausdrückliche Richtlinienänderung.

Ablehnungen landen in `cron_jobs.skipped_tools` und erscheinen sowohl in der Benachrichtigung als auch im Anhang des Berichts.

### Zuverlässigkeit und Kostenkontrolle

| Mechanismus | Hinweis |
|-------------|---------|
| Schrittbudget | `max_steps` je Aufgabe (Standard 12, im Formular änderbar) |
| Gesamtlaufzeit | `timeout_seconds` je Aufgabe (Standard 600 s); eine Zeitüberschreitung zählt als Fehler |
| Automatische Deaktivierung | 3 Fehler in Folge deaktivieren die Aufgabe, damit ein defekter Job nicht ewig alarmiert |
| Manuelles Zurücksetzen | Speichern der Aufgabe im Panel setzt `fail_count` auf 0 |

### Berichte: eine einzelne HTML-Datei

Jeder Lauf erzeugt **eine in sich geschlossene HTML-Datei** — das Artefakt, das man öffnet oder herunterlädt:

```
<Datenverzeichnis>/reports/<Aufgabenname>/<Aufgabenname>-<Zeitstempel>.html   <- der Bericht (in sich geschlossen)
<Datenverzeichnis>/reports/<Aufgabenname>/<Aufgabenname>-<Zeitstempel>.md     <- derselbe Inhalt als Markdown
```

`<Datenverzeichnis>` ist das Datenverzeichnis des Panels (`appstore_data_dir`, Standard `/var/lib/opsmini/data`). **Jedes erzeugte Artefakt liegt darunter, ein Unterverzeichnis pro Funktion** — Berichte in `reports/`; künftige Generatoren fügen einfach Nachbarverzeichnisse hinzu.

„In sich geschlossen" heißt: Stile inline, kein Skript, kein externes Stylesheet, keine Web-Schrift, keine Bilder. Ein Bericht lässt sich daher von einem USB-Stick, als Mailanhang oder von einem beliebigen statischen Host aus darstellen. Das Panel bündelt bewusst **keine CJK-Schrift** (sie würde das einzelne Binary um mehr als 10 MB vergrößern), Berichte nutzen also die Systemschriften des Lesers, die jeder Desktop- und Mobilbrowser mitbringt.

Inhalt: eine Ausführungsübersicht (Status / Schritte / Werkzeugaufrufe), die Aufgabenanweisung, das Fazit und ein **Anhang: Ausführungsprotokoll** mit einer Zeile je Werkzeugaufruf — Argumente und Beobachtung nebeneinander, Status einer von „erledigt / abgelehnt / hart blockiert / fehlgeschlagen".

Jede Aufgabenzeile bietet zwei Einstiegspunkte:

- **Öffnen** — `GET /ai/agent/reports/view`, `Content-Disposition: inline`, dargestellt in einem neuen Browser-Tab;
- **Herunterladen** — `GET /ai/agent/reports/download`, `Content-Disposition: attachment`.

Der Text ist vom Modell geschriebenes Markdown, vermischt mit Host-Daten (Logauszüge, Kommandozeilen von Prozessen, Konfigurationswerte). Der Renderer maskiert das gesamte Dokument **zuerst** und wendet erst danach Inline-Regeln an; Ziele wie `javascript:` werden nicht verlinkt, ein `<script>` in einer Logzeile erscheint also als Text. Zusätzlich wird eine strikte CSP als zweite Verteidigungslinie mitgesendet.

Für PDF drucken Sie den geöffneten Bericht im Browser („Drucken → Als PDF speichern"); das Stylesheet enthält Druckregeln, das Ergebnis entspricht dem Bildschirm.

### Auch Ad-hoc-Gespräche exportieren

Jedes Gespräch im AI-Assistenten lässt sich als derselbe HTML-Bericht exportieren: `GET /ai/agent/sessions/:id/report` (standardmäßig HTML, `?format=md` für Markdown, `?download=1` erzwingt einen Download). Auch nie archivierte Sitzungen liefern damit einen lesbaren Bericht.

### Gemeinsamer Benachrichtigungspfad

KI-Aufgaben und Alarme teilen sich einen `Notifier` (`internal/service/notify.go`) für die Meldung im Panel, E-Mail sowie WeCom-/DingTalk-/Feishu-Gruppenroboter. Eine leere Kanalliste bedeutet die Meldung im Panel.
