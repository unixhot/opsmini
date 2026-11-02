# Protokoll-Pipeline

OpsMini enthält einen integrierten Log-Sammel-Agent, der **mit der Filebeat-Konfiguration kompatibel** ist. Er sammelt Protokolle auf dem Host gemäß der Aufgabenkonfiguration, verarbeitet sie und liefert sie an Elasticsearch / Kafka / Logstash. Seine Position entspricht dem „filebeat auf einem verwalteten Host“, es ist jedoch kein separater Prozess erforderlich.

## Standardmäßig deaktiviert

Das muss zuerst klargestellt werden: **Die Standardinstallation sammelt keinerlei Protokolle**. Keine Goroutine, kein Verzeichnis-Scan, kein Speicherverbrauch.

Dreistufige Aktivierung – sobald eine der Bedingungen erfüllt ist, wird gestartet:

| Methode | Auslösebedingung | Anwendungsfall |
|---|---|---|
| Aufgabendatei | Im Verzeichnis `tasks.d` erscheint eine beliebige `.yml` / `.yaml` | Manuelle Konfiguration der Einzel-Host-Sammlung |
| Expliziter Schalter | `logpipeline.enabled: true` in `config.yaml` | Erzwungene Aktivierung (auch ohne aktuelle Aufgaben) |
| Plattform-Auslieferung | Empfang von `LogTask`, das von OpsAnt ausgeliefert und auf Datenträger geschrieben wird | Zentrale Verwaltung mehrerer Hosts |

Umgekehrt gilt: Wenn `enabled: false` und keine Aufgabendatei vorhanden ist, ist die Pipeline vollständig inaktiv – das ist der Standardzustand im Szenario „Einzel-Host-Eigennutzung".

## Verzeichnisstruktur

```
<Datenverzeichnis>/logs/
├── main.yml          # Hauptkonfiguration (filebeat-Form: output + globale Parameter)
├── tasks.d/          # Erfassungsaufgaben, eine Aufgabe pro Datei
│   ├── nginx.yml
│   └── app-1024.yml
├── registry.db       # Leseposition jeder Datei (inode + offset)
└── spool/            # Festplattenwarteschlange bei Ausgabefehlern
```

`main.yml` ist für „wohin senden" zuständig, `tasks.d/*.yml` für „was erfassen". Diese Aufteilung entspricht `filebeat.yml` + `inputs.d/` von filebeat, daher können bestehende filebeat-Konfigurationen direkt aufgeteilt und verwendet werden.

## Wie Aufgaben geschrieben werden

**Variante A (empfohlen)**: Die Aufgabe beschreibt nur die Erfassung, die Ausgabe wird einheitlich von `main.yml` bestimmt.

```yaml
# tasks.d/nginx.yml
- type: filestream
  id: nginx-access
  paths:
    - /var/log/nginx/access.log
  fields:
    service: nginx
    env: prod
  parsers:
    - ndjson:
        target: ""
        add_error_key: true
  processors:
    - drop_fields:
        fields: ["agent.ephemeral_id", "ecs.version"]
    - add_fields:
        target: ""
        fields:
          cluster: bj-prod
```

**Variante B**: Die Aufgabendatei bringt `output` mit, für „verschiedene Aufgaben an verschiedene Orte senden". Dies ist eine Erweiterung von OpsMini gegenüber filebeat – bei filebeat werden dafür mehrere Instanzen benötigt.

```yaml
output.elasticsearch:
  hosts: ["http://10.0.0.5:9200"]
  index: "app-%{+yyyy.MM.dd}"
```

Beide Varianten werden akzeptiert, B hat eine höhere Priorität als `main.yml`.

## Hot-Reload

`tasks.d` gleicht alle 10 Sekunden ab:

- **Neue** Aufgabendatei → nach dem Parsen wird die Erfassung gestartet; bei einem Parsing-Fehler wird die laufende alte Version **nicht überschrieben**, um zu vermeiden, dass „ein falsch geändertes Feld die gesamte Erfassung stoppt“
- **Geänderte** Aufgabendatei → Neuaufbau nur, wenn sich der Inhalts-Hash geändert hat; das wiederholte Ausliefern desselben Inhalts löst keinen Neustart aus (ein Neustart würde den Multiline-Puffer verlieren und alle Dateien erneut öffnen)
- **Gelöschte** Aufgabendatei → die Aufgabe wird gestoppt

Der Aufgabendateiname darf nur `*.yml` / `*.yaml` sein, darf keinen Pfad enthalten und nicht mit einem Punkt beginnen – der Dateiname ist das einzige Feld, das potenziell remote zum Schreiben aus dem Verzeichnis heraus genutzt werden könnte, daher wird er direkt strikt eingeschränkt.

## Unterstützte Prozessoren

31 Filebeat-Prozessoren, die gängige Bereinigungsanforderungen abdecken:

| Kategorie | Prozessoren |
|---|---|
| Feld hinzufügen/löschen/ändern | `add_fields` `add_tags` `drop_fields` `include_fields` `rename` `copy_fields` `move_fields` `truncate_fields` |
| Typ und Parsing | `convert` `decode_json_fields` `decode_csv_fields` `decode_duration` `dissect` `syslog` `decode_xml` `decode_base64_field` `decompress_gzip_field` `urldecode_fields` `extract_array` |
| Bedingungen und Flusskontrolle | `drop_event` `script` |
| Zeit und Kennung | `timestamp` `add_id` `fingerprint` |
| Metadaten | `add_host_metadata` `add_observer_metadata` `add_locale` `add_network_direction` |
| Netzwerk und Domain | `community_id` `registered_domain` |
| Textersetzung | `replace` |

## Ausgabe

| Typ | Beschreibung |
|---|---|
| `elasticsearch` | Unterstützt Index-Template-Variablen, Benutzername/Passwort, Batch-Größe und Timeout |
| `kafka` | Unterstützt SASL-Authentifizierung, Topic-Variablen, Partitionierungsstrategie |
| `logstash` | Lumberjack-v2-Protokoll mit TLS (sowohl `logstash` als auch `lumberjack` werden erkannt) |

Wenn keine Ausgabe konfiguriert ist, sammelt die Aufgabe weiterhin normal, aber die Zustellung schlägt fehl – das Panel zeigt den Fehler im Pipeline-Status an und verwirft ihn nicht stillschweigend.

## Vorgehensweise ohne Datenverlust

Die Zustell-Semantik ist **at-least-once**:

1. Einen Stapel Protokolle lesen → zustellen
2. **Erst nach erfolgreicher Zustellung** den Offset in `registry.db` vorrücken
3. Zustellung fehlgeschlagen → in `spool/` schreiben (Standard-Obergrenze 512MB) → Backoff-Wiederholung
4. Prozessabsturz → nach Neustart erneutes Senden ab dem zuletzt bestätigten Offset, möglicherweise Duplikate, aber kein Verlust

Transaktional ist ein doppelter Stapel Protokolle in der Regel harmlos (auf der ES-Seite kann der `fingerprint`-Prozessor für Idempotenz sorgen), während verlorene Protokolle nicht wiederherstellbar sind – daher wurde Ersteres gewählt.

## Echtzeitvorschau

Im Panel **Protokoll → Pipeline**-Tab ist Folgendes sichtbar:

- Ob die Pipeline läuft, der aktuelle Ausgabetyp, Ausgabefehler
- Die Anzahl der erfassten Dateien pro Aufgabe, die Leserate, der Zeitpunkt des letzten Ereignisses
- Der Spool-Rückstau
- Konfigurationswarnungen (unbekannte Felder, nicht existierende Pfade usw.)

Durch Auswahl einer Aufgabe kann man **die letzten 20 Ereignisse in Echtzeit in der Vorschau anzeigen** und sehen, wie es nach der Prozessorkette aussieht – ohne erst in ES nachschauen zu müssen und ohne die Konfiguration zu ändern und neu zu starten. Das ist der schnellste Weg, um einen Prozessor zu debuggen.

## Warum Filebeat-Kompatibilität

Drei Gründe:

1. **Migrationskosten**: Bestehende ES-Index-Templates, ILM-Richtlinien und Kibana-Dashboards bleiben unverändert, `agent.type` kann weiterhin auf `filebeat` gesetzt werden, um sie direkt wiederzuverwenden
2. **Betriebslesbarkeit**: Die Erfassungskonfiguration ist das YAML, mit dem der Betrieb am vertrautesten ist, kein neues DSL muss erlernt werden
3. **Keine neuen Abhängigkeiten**: Die Funktionalität ist in das einzelne Binary integriert, filebeat muss nicht zusätzlich für die Protokollerfassung installiert werden

## Was nicht abgedeckt wird

Ehrliche Abgrenzung, um Missverständnisse zu vermeiden:

- **Kein module / autodiscover** (Nginx-Module, Kubernetes-Auto-Discovery): Ein Einzelhost-Panel hat keinen Container-Orchestrierungskontext
- **Keine ILM- und Index-Template-Verwaltung**: Das ist Sache der ES-Seite
- **Keine hostübergreifende Aggregation**: Die Multi-Host-Erfassung wird von OpsAnt über verteilte Aufgaben zentral verwaltet, OpsMini fungiert nur als Ausführungsende
- **Nicht alle Filebeat-Ausgaben werden unterstützt** (Middleware außer Logstash Beat, Redis, Kafka): Nur ES / Kafka / Logstash drei Varianten

## Konfigurationsreferenz

```yaml
# config.yaml
logpipeline:
  enabled: false            # Hard switch; bei true wird auch ohne Aufgabendatei gestartet
  tasks_dir: ""             # Standard <Datenverzeichnis>/logs/tasks.d
  config_file: ""           # Hauptkonfiguration, Standard <Datenverzeichnis>/logs/main.yml
  strict: false             # Bei true führen unbekannte Felder in Aufgaben direkt zu einem Fehler statt zu einer Warnung
  agent_type: opsmini       # Wird in agent.type geschrieben; auf filebeat setzen, um vorhandene Vorlagen wiederzuverwenden
```

In Multi-Host-Szenarien werden diese Aufgabendateien von OpsAnt über gRPC ausgeliefert, der Betreiber konfiguriert sie auf der Plattform, auf der Host-Seite ist keine manuelle Bearbeitung erforderlich.

## Beziehung zu anderen Modulen

| Modul | Beziehung |
|---|---|
| [Autonomer Agent (Event-Center)]](autonomous-agent.md) | Log-Boom-Collector und Log-Pipeline sind zwei verschiedene Dinge: Ersterer prüft nur „ob es ein anomalies Wachstum gibt", Letztere ist dafür verantwortlich, Logs an die Log-Plattform zu liefern |
| [KI-Assistent]](ai-assistant.md) | Die `search_logs`-Tools des KI-Assistenten lesen direkt lokale Log-Dateien und sind nicht von der Pipeline abhängig |
| [Konfigurationsleitfaden · Logs und Daten]](../configuration/storage.md) | Datenverzeichnis und Aufbewahrungsrichtlinie |
