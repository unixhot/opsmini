# AI-Assistent

OpsMini enthält integrierte AI-LLM-Betriebsfähigkeiten. Der AI-Assistent ist nicht bloß „ein Chatbot, der Systeminformationen lesen kann", sondern ein **Agent**: Auf Ihre Frage hin plant er selbstständig, ruft Werkzeuge für echte Daten auf, beobachtet die Ergebnisse und arbeitet weiter, bis er zu einem Schluss kommt. Jede risikoreiche Aktion braucht jedoch weiterhin zuerst Ihre Freigabe.

## Fähigkeiten

- **Autonome mehrstufige Fehlersuche**: Beschreiben Sie das Symptom, und die AI verkettet selbst mehrere Werkzeugaufrufe (Kennzahlen prüfen → Prozesse ansehen → Logs lesen), bevor sie antwortet — statt aus dem Gedächtnis zu antworten
- **Sichtbare Gedankenkette**: Jeder Werkzeugaufruf erscheint als Karte, die Sie aufklappen können, um Werkzeugname, Argumente und Rohausgabe zu sehen; der Schluss ist damit nachvollziehbar
- **29 integrierte Betriebswerkzeuge**: Systemkennzahlen, Prozesse, Ports, Festplatten, Dateien, Logs, Dienste, Container, Sicherheit und Zeitpläne
- **Abgestufte Autorisierung mit Hochrisiko-Bestätigung**: Nur lesende Aktionen laufen von selbst; risikoreiche Aktionen öffnen einen Dialog und warten auf Sie
- **MCP in beide Richtungen**: Externe MCP-Server als Werkzeugquelle nutzen und das eigene Werkzeugset als MCP-Server für Claude Desktop / Cursor und andere veröffentlichen
- **Geplante KI-Aufgaben**: z. B. jeden Morgen automatisch eine Host-Prüfung, die einen einteiligen HTML-Bericht erzeugt und an Ihre Benachrichtigungskanäle sendet
- **Persistente Sitzungen**: Gespräche werden gespeichert, sodass ein Neuladen der Seite oder ein Prozessneustart die Gedankenkette wiedergeben kann; lange Verläufe werden automatisch komprimiert
- **Skills (Skill Hub)**: Skill-Pakete im Marktplatz suchen, installieren und verwalten

## Werkzeuge und Risikostufen

Alles, was die AI tun kann, ist als **Werkzeug** definiert, und jedes Werkzeug trägt eine Risikostufe:

| Stufe | Bedeutung | Standardverhalten |
|-------|-----------|-------------------|
| **L0 nur lesend** | Liest Informationen, ändert nichts | Läuft unbeaufsichtigt |
| **L1 risikoarm schreibend** | Begrenzte Schreibzugriffe (temporäre Dateien aufräumen, Dienst neu starten, ...) | Standardmäßig bestätigungspflichtig; ein Administrator kann einzelne Werkzeuge ausnehmen |
| **L2 risikoreich schreibend** | Befehle ausführen, Dateien schreiben oder löschen, SSH- oder Firewall-Konfiguration ändern, ... | **Jedes Mal menschliche Freigabe nötig**; kann nie auf automatisch gestellt werden |
| **L3 verboten** | Festplatten formatieren, `rm -rf /`, Firewall-Regeln leeren, ... | **Vor der Ausführung blockiert**, selbst mit Freigabe |

Beispielwerkzeuge je Stufe:

| Stufe | Beispiele |
|-------|-----------|
| L0 nur lesend | `sys_overview`, `list_processes`, `list_ports`, `list_disks`, `dir_usage`, `read_file`, `search_logs`, `query_metrics`, `docker_ps`, `security_status` |
| L1 risikoarm schreibend | `clean_temp`, `clean_logs`, `service_control`, `docker_control`, `install_skill` |
| L2 risikoreich schreibend | `exec_command`, `write_file`, `delete_path`, `modify_sshd`, `compress_dir`, `cron_write`, `kill_process`, `firewall_rule` |

Werkzeuge folgen außerdem dem RBAC des Panels: Eine nur lesende Rolle sieht keine Werkzeuge, die Schreibrechte benötigen, und L2-Werkzeuge verlangen zusätzlich die Berechtigung `ai.agent.write`.

Der vollständige Bestand steht unter **„AI-Assistent → Werkzeugberechtigungen"**, wo Sie auch die Richtlinie je Werkzeug ändern können (**automatisch / bestätigen / deaktiviert**).

## Menschliche Freigabe

Will die AI eine risikoreiche Aktion ausführen, tut sie das nicht einfach:

1. Im Gespräch erscheint eine **Bestätigungskarte** mit Werkzeugname, vollständigen Argumenten und dem auszuführenden Befehl (`exec_command` zeigt beispielsweise die ganze Kommandozeile);
2. Klicken Sie auf **Freigeben**, läuft die Aktion wirklich, und die Ausgabe geht zur weiteren Schlussfolgerung an die AI zurück;
3. Klicken Sie auf **Ablehnen**, läuft nichts — die AI erhält den Grund, erklärt die Auswirkungen und bietet eine Alternative an.

Freigabeanfragen laufen standardmäßig nach 10 Minuten ab. Freigaben wie Ablehnungen landen im Audit-Protokoll.

## Sicherheitsmechanismen

- **Erst scannen, dann ausführen**: Jeder Werkzeugaufruf durchläuft zuerst den L3-Verbotsscan; ein Treffer wird abgelehnt und der Grund an das Modell zurückgemeldet
- **Selbstschutz des Panels**: Alles, was das Panel selbst beschädigen würde (Datenverzeichnis, lauschender Port), wird abgelehnt
- **Herabstufung ohne Aufsicht**: Bei einer geplanten Aufgabe gibt es niemanden, der „Freigeben" klickt, daher werden L2-Aktionen automatisch abgelehnt und im Bericht vermerkt, während die Aufgabe ihre übrigen Schritte beendet
- **Vollständige Prüfspur**: Werkzeugaufrufe, Freigaben und Ablehnungen landen alle im Audit-Protokoll
- **Hauptschalter**: Wird „AI-Assistent aktivieren" ausgeschaltet, lehnen die AI-Endpunkte direkt ab

> Produktentscheidung: Es gibt keinen unbeaufsichtigten Root-Agenten. Zielbild ist **„autonome Planung + abgestufte Autorisierung"** — die KI untersucht und berät, der Mensch autorisiert alles Destruktive.

## Geplante KI-Aufgaben

Eine „KI-Aufgabe" ist kein eigener Einstiegspunkt, sondern die **vierte Art geplanter Aufgabe**, neben „Panel-Aufgaben / System-crontab / Benutzer-crontab". Zeitplanung, Aktivieren/Deaktivieren, letzte Ausführungszeit und Fehleralarme werden damit vollständig wiederverwendet.

Konfiguriert wird sie unter **„Geplante Aufgaben → Neue Aufgabe → Aufgabentyp: Agent-Aufgabe"**:

- **Prompt**: was die AI tun soll. Fünf Vorlagen sind eingebaut (Host-Prüfung / Sicherheitsprüfung / Speicherplatzprüfung / Dienststatusprüfung / Log-Anomalieprüfung) — ein Klick füllt das Feld
- **Zeitplan**: ein Standard-cron-Ausdruck, mit gängigen Vorlagen (täglich um 08:00, ...)
- **Maximale Schritte**: begrenzt die Zahl der Werkzeugaufrufe und damit Laufzeit und Tokenkosten
- **Benachrichtigungskanäle**: Panel / E-Mail / WeCom / DingTalk / Feishu

Jede Zeile der Aufgabenliste bietet **Ausführen** (einmal sofort) und **Bericht**. Eine Aufgabe, die dreimal in Folge scheitert, wird automatisch deaktiviert, damit ein defekter Job nicht ewig alarmiert.

## Berichte

Jeder Lauf erzeugt einen **einteiligen HTML-Bericht** — Stile inline, kein Skript, keine externen Abhängigkeiten. Er lässt sich überall öffnen und übersteht einen Mailanhang oder einen USB-Stick unbeschadet.

```
<Datenverzeichnis>/reports/<Aufgabenname>/<Aufgabenname>-<Zeitstempel>.html     <- der Bericht
<Datenverzeichnis>/reports/<Aufgabenname>/<Aufgabenname>-<Zeitstempel>.md       <- derselbe Inhalt als Markdown
```

Das Datenverzeichnis wird unter **Panel „Einstellungen → Datenspeicherung"** festgelegt (Standard `/var/lib/opsmini/data`). Jedes erzeugte Artefakt liegt darunter, ein Unterverzeichnis pro Funktion.

Inhalt: eine Ausführungsübersicht (Status / Schritte / Werkzeugaufrufe), die Aufgabenanweisung, das Fazit und ein **Anhang: Ausführungsprotokoll** mit einer Zeile je Werkzeugaufruf — Argumente und Beobachtung nebeneinander, Status einer von „erledigt / abgelehnt / hart blockiert / fehlgeschlagen".

Jede Aufgabenzeile hat zwei Einstiegspunkte:

- **Öffnen** — stellt den Bericht direkt in einem neuen Browser-Tab dar, ohne Download-Dialog
- **Herunterladen** — speichert ihn als `.html`-Datei

Für PDF drucken Sie den geöffneten Bericht im Browser („Drucken → Als PDF speichern"); das Stylesheet enthält Druckregeln, das Ergebnis entspricht dem Bildschirm.

## Unterstützte LLM-Anbieter

| Provider | Beschreibung |
|----------|--------------|
| `openai` | OpenAI-kompatible Schnittstelle |
| `deepseek` | DeepSeek |
| `qwen` | Tongyi Qianwen (DashScope-kompatible Schnittstelle) |
| `ollama` | Lokales Ollama-Deployment |

Jeder Dienst, der das OpenAI-Protokoll `chat/completions` spricht, funktioniert. Werkzeugaufrufe benötigen ein Modell mit Function-Calling-Unterstützung; Modelle ohne diese fallen automatisch auf ein normales Gespräch zurück.

## Konfiguration

Einheitlich in **Panel „Einstellungen → AI-LLM-Anbindung"** konfigurieren (der frühere `ai`-Abschnitt in `config.yaml` wurde hierher verlagert, zur Laufzeit sofort wirksam):

- AI-Assistent aktivieren (Hauptschalter)
- Modell-Anbieter / Modellname / API-Key / API-Adresse
- Schalter für Operationen in natürlicher Sprache / intelligente Log-Analyse / intelligente Alarm-Diagnose

> Konfigurationspriorität: Umgebungsvariable `OPSMINI_AI_KEY` > Panel-Einstellungen > verbliebene `ai.*` in der Konfigurationsdatei. Es wird empfohlen, den Schlüssel in einer Umgebungsvariable zu speichern, um eine Speicherung auf der Festplatte zu vermeiden. Details siehe [Panel-Einstellungen](../configuration/panel-settings.md#ai-config).

Nach der Konfiguration zeigt der Laufzeitbereich oben auf der Seite „AI-Assistent → Werkzeugberechtigungen", ob das Modell bereit ist (`model_ready`).

## MCP-Verwaltung

### Als Client: externe Werkzeuge nutzen

Unter „AI-Assistent → MCP" lassen sich MCP-Server erstellen, lesen, ändern und löschen, um die Fähigkeiten und Datenquellen der AI zu erweitern. Zwei Übertragungsarten werden unterstützt:

- **Befehl (stdio)**: z. B. ein lokal laufender MCP-Serverprozess
- **HTTP (Streamable HTTP)**: ein entfernter MCP-Server, mit eigenen Anfrage-Headern

Nach dem Verbinden erscheinen externe Werkzeuge als `mcp__<Servername>__<Werkzeugname>` im Bestand; die Risikostufe ergibt sich aus dem vom Server angegebenen Nur-Lese-Hinweis. Verbindungen werden alle zwei Minuten abgeglichen, Konfigurationsänderungen brauchen also keinen Neustart.

### Als Server: andere KIs dürfen OpsMini steuern

Das Panel kann sein eigenes Werkzeugset auch veröffentlichen, damit MCP-Clients wie Claude Desktop, Cursor oder OpsAnt es aufrufen:

- Der Endpunkt ist `/mcp` (Streamable HTTP), authentifiziert mit einem **Zugriffstoken**, das Sie unter „AI-Assistent → Werkzeugberechtigungen" einsehen, kopieren und erneuern können
- **Standardmäßig werden nur nur-lesende und risikoarme Werkzeuge veröffentlicht.** Risikoreiche Werkzeuge (Befehl ausführen, Datei schreiben, ...) bleiben verborgen, weil ein externer Client den Bestätigungsdialog des Panels nicht anzeigen kann; aktivieren Sie auf derselben Seite „Riskante MCP-Aktionen erlauben", wenn Sie sie wirklich brauchen
- Die L3-Verbotsregeln und der Selbstschutz des Panels gelten auch für entfernte Aufrufe

## Skills (Skill Hub)

Der integrierte Tab „Skills" ist an den SkillHub-Skillmarkt angebunden:

- Skills suchen und per Ein-Klick installieren
- Lokale Skills hochladen, Skills manuell erstellen
- Installierte Skills aktivieren/deaktivieren, löschen
