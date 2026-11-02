# Plugin-System

Der Kernel erledigt nur „die Dinge, die alle Hosts benötigen": Erfassung, Audit, Agent-Laufzeit, Guardrails. Geschäftsfähigkeiten (Metriken eines bestimmten Middleware-Produkts, die Analyse einer bestimmten Art von Protokollen, die Aktionsanbindung an eine bestimmte Plattform) werden mit **Plugins** erweitert.

Der Zweck dieses Vorgehens ist nicht das Streben nach architektonischer Schönheit, sondern die Vermeidung zweier konkreter Dinge: **unbegrenztes Anwachsen des Binaries** sowie **die Notwendigkeit, für eine neue Fähigkeit eine neue Version zu veröffentlichen**.

## Plugins sind eigenständige Prozesse, kein Go-Plugin

- Das Go-Paket `plugin` erfordert eine vollständig identische Build-Umgebung zum Kompilierungszeitpunkt, ein Versionswechsel führt zwangsläufig zum Absturz, daher direkt ausgeschlossen
- Plugins sind **eigenständige ausführbare Dateien**, das Panel startet sie als Unterprozess und kommuniziert über einen Unix-Socket mittels **zeilengetrenntem JSON**
- Ein Plugin-Absturz beeinträchtigt das Panel nicht: Der Prozess-Exit wird protokolliert, gemäß der Neustartstrategie neu gestartet, bei wiederholtem Fehlschlag als failed markiert und die Wiederholung eingestellt

## Paketformat

```
foo-collector-1.2.0-linux-amd64.tar.gz
├── plugin.yaml          # Manifest
├── bin/plugin           # Einstiegspunkt (ausführbar)
├── SIGNATURE            # ed25519-Signatur (signiert sha256 des gesamten Pakets)
└── README.md            # optional
```

Manifest-Beispiel:

```yaml
apiVersion: opsmini.plugin/v1
id: foo-collector              # global eindeutig, ^[a-z0-9][a-z0-9-]{2,40}$
name: Foo 采集器
version: 1.2.0                 # semver
vendor: 速云科技
description: 采集 Foo 服务的队列深度与消费速率
kind: collector                # collector | action | panel
entry: bin/plugin              # relativer Pfad innerhalb des Pakets
platform: [linux/amd64, linux/arm64]

capabilities:                  # Fähigkeitsdeklaration, dient zugleich als Audit-Grundlage
  - metrics
  - read:/etc/foo
  - exec:systemctl
  - net:outbound

runtime:
  memoryLimitMB: 128
  cpuQuotaPercent: 20
  restartPolicy: on-failure    # always | on-failure | never

requires:
  opsmini: ">=1.4.0"           # Anforderung an die Host-Agent-Version
```

Validierungsreihenfolge bei der Installation (bei jedem Fehlschlag wird abgelehnt, es wird kein „halbfertiges Plugin" installiert):

1. Paket-sha256 (serverseitig berechnet, kann mit der vom Uploader angegebenen Prüfsumme abgeglichen werden)
2. ed25519-Signatur
3. `apiVersion`-Kompatibilität
4. Ob `platform` zum aktuellen Host passt
5. Gültigkeit der Manifest-Felder, Versionsanforderungen

## Drei Kanäle

Das Plugin und der Kernel kommunizieren ausschließlich über Socket mit vier Nachrichtentypen, alle als einzeiliges JSON:

| Nachricht | Richtung | Zweck |
|---|---|---|
| `hello` | Plugin → Panel | Start-Handshake, deklariert Identität, Version und Fähigkeiten |
| `metric` | Plugin → Panel | Meldet Metriken (fließen in Überwachung und Alarmregeln ein) |
| `event` | Plugin → Panel | Meldet Ereignisse (können als Signale ins Event-Center übergehen) |
| `heartbeat` | Plugin → Panel | Liveness- und Selbstprüfungsstatus |

Das Plugin greift nicht direkt auf die Datenbank zu, ruft keine Panel-Schnittstellen direkt auf und erbt keine Sitzung des Panels.

## Lebenszyklus

```
Installation (Paket hochladen → Verifizieren → Entpacken nach releases/<version>/ → current-Symlink zeigt darauf)
   ↓
Aktivieren → Prozess starten → auf hello-Handshake warten → in running wechseln
   ↓
Betrieb → Absturz/Beendigung → gemäß restartPolicy behandeln, Neustartzähler und letzten Fehler protokollieren
   ↓
Deaktivieren → SIGTERM → auf Beendigung warten → Status stopped
   ↓
Upgrade → neue Version entpacken → current-Symlink umschalten → Neustart (altes Versionsverzeichnis bleibt erhalten, Rollback möglich)
   ↓
Löschen → Prozess stoppen → Programmdateien entfernen (Datenverzeichnis bleibt erhalten)
```

## Berechtigungsgrenzen: Klarstellen, was umgesetzt ist

Dieser Abschnitt muss ehrlich sein, sonst wird „Plugin-Sicherheit" zu einer leeren Floskel.

**Umgesetzt:**

- Plugins sind eigenständige Prozesse; ein Absturz bringt das Panel nicht zu Fall
- Die Installation erfordert eine Signaturprüfung (außer wenn der unsigned-Modus explizit aktiviert ist), und es werden Version, Plattform und Manifest geprüft
- Installation, Aktivierung, Deaktivierung und Löschung werden vollständig im Audit-Protokoll erfasst
- Plugins kommunizieren über einen Socket und können nicht direkt auf die Panel-Datenbank zugreifen

**In der aktuellen Version nicht umgesetzt (nicht annehmen, dass es existiert):**

- **Kein Wechsel zu einem eigenständigen System-Benutzer**: Plugins laufen unter dem Benutzer des Panel-Prozesses, üblicherweise `root`
- **Keine cgroup-Ressourcenbegrenzung**: `memoryLimitMB` / `cpuQuotaPercent` im Manifest sind derzeit nur Deklarationen, nicht erzwungen
- **`capabilities` nicht erzwungen**: Es dient als Deklarations- und Audit-Grundlage, nicht als kernelbasierte Whitelist
- **Systemaufrufe innerhalb von Plugins können nicht auditiert werden**

Die Schlussfolgerung lautet also: **Eine Signatur belegt „vertrauenswürdige Herkunft", nicht „sicheres Verhalten"**. Installieren Sie nur Plugins aus Quellen, denen Sie vertrauen; prüfen Sie bei Drittanbieter-Plugins den Code selbst.

## Installationsvoraussetzungen (bei der aktuellen Version zu beachten)

Bei der Installation wird der Reihe nach geprüft: `sha256` → Signatur → Manifest. In der Standardkonfiguration ist **weder ein integrierter öffentlicher Schlüssel vorhanden noch der unsigned-Modus aktiviert**, daher wird ein direkter Upload abgelehnt mit der Meldung „Kein Plugin-Public-Key konfiguriert und unsigned-Modus nicht aktiviert".

Zwei Lösungswege, wählen Sie einen (nach dem Eintragen in die Einstellungen ist **ein Neustart des Panels erforderlich**, damit die Änderung wirksam wird, da der öffentliche Schlüssel beim Start gelesen wird):

```bash
# 方式一：配置 ed25519 公钥（推荐，正式环境）
curl -X POST http://127.0.0.1:8888/api/v1/settings \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"plugin_public_key":"<hex 编码的 32 字节公钥>"}'

# 方式二：开发/内网环境临时允许未签名包
curl -X POST http://127.0.0.1:8888/api/v1/settings \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"plugin_allow_unsigned":"true"}'
```

Nach der Änderung `systemctl restart opsmini`, dann zurück zu „Panel-Einstellungen → Plugins" und hochladen.

> Die aktuelle Panel-Version bietet noch keine Eingabemaske für diese beiden Einstellungen; sie können nur über die obige Schnittstelle oder durch direktes Bearbeiten der Einstellungstabelle vorgenommen werden. Dies wird in der nächsten Version ergänzt.

## Panel-Betrieb

**Panel-Einstellungen → Plugins**:

| Aktion | Beschreibung |
|---|---|
| Upload-Installation | `*.tar.gz`-Paket auswählen, maximal 64MB |
| Aktivieren / Deaktivieren | Sofort wirksam, kein Neustart des Panels erforderlich |
| Neustart | Einzelnes Plugin neu starten (z. B. nach Änderung seiner Konfiguration) |
| Löschen | Programmdateien entfernen, Datenverzeichnis bleibt erhalten |
| Ereignisse | Die zuletzt vom Plugin gemeldeten Metriken und Ereignisse anzeigen |

In der Liste sind für jedes Plugin sichtbar: Name, Version, Typ, Status (running / stopped / failed / installed), Anzahl der Neustarts, letzter Fehler.

## Statusbeschreibung

Der Mechanismus ist vollständig, **das Ökosystem ist derzeit leer** – v1.4.0 enthält kein offizielles Plugin-Paket. Das bedeutet, dass die Plugin-Seite nach der Installation von OpsMini leer ist; dies ist der erwartete Zustand und keine Fehlfunktion.

Geplante offizielle Plugin-Richtungen (nicht veröffentlicht): erweiterte Inspektionsrunden für Sicherheits-Baselines, Metrik-Erfassung für spezifische Middleware, Regelpakete für Protokoll-Parsing, Anbindung an Aktionen von Drittanbieter-Plattformen.

## Beziehung zu anderen Modulen

| Modul | Beziehung |
|---|---|
| [Autonomer Agent (Event-Center)]](autonomous-agent.md) | Die vom Plugin gemeldeten `event` können in Signale umgewandelt und in das Event-Center eingespeist werden, um an Aggregation und Bearbeitung teilzunehmen |
| [Dashboard und Überwachung]](dashboard-monitoring.md) | Die vom Plugin gemeldeten `metric` können von Alarmregeln verwendet werden |
| [Protokoll-Pipeline]](log-pipeline.md) | Fähigkeiten zur Protokollanalyse können sowohl als Processor in der Pipeline als auch als Plugin umgesetzt werden; Ersteres ist leichter, Letzteres kann unabhängig aktualisiert werden |
| [Host-Sicherheit]](security.md) | Offizielle Sicherheits-Plugins sind eine Erweiterungsrichtung der Host-Sicherheit |
