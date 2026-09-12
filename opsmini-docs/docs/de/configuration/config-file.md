# Hauptkonfigurationsdatei

OpsMini verwendet eine YAML-Konfigurationsdatei. Nach der offiziellen Ein-Klick-Installation liegt sie standardmäßig unter `/data/opsmini/config.yaml`; beim Betrieb aus dem Quellcode standardmäßig unter `configs/config.yaml`. Beide lassen sich mit dem Parameter `-config` angeben. Existiert die Konfigurationsdatei nicht, startet das Programm mit den eingebauten Standardwerten.

Das offiziell mitgelieferte Beispiel wurde auf eine **minimal lauffähige Konfiguration** reduziert; die übrige Laufzeitkonfiguration (AI, Agent-Token, Monitoring-Authentifizierung usw.) wurde auf die Seite „Einstellungen" des Panels verlagert.

```yaml
# OpsMini-Agent-Konfigurationsdatei
server:
  host: "0.0.0.0"        # Lauschadresse
  port: 8888              # Lauschport
  secret_entry: ""        # Pfad-Präfix des Sicherheitseingangs, z. B. /opsmini_panel (leer = deaktiviert)

database:
  path: "opsmini.db"      # Pfad der SQLite-Datendatei

jwt:
  access_ttl_seconds: 900     # Gültigkeit des Access-Tokens (15 Minuten)
  refresh_ttl_seconds: 604800 # Gültigkeit des Refresh-Tokens (7 Tage)

log:
  level: error               # Log-Level: info / warn / error
  path: ""                   # Pfad der Log-Datei; leer = Ausgabe nur nach stdout (systemd journal)
```

## Beschreibung der Konfigurationselemente

### server

| Parameter | Beschreibung | Standard |
|------|------|------|
| `host` | Lauschadresse; `0.0.0.0` bedeutet alle Netzwerkkarten | `0.0.0.0` |
| `port` | Lauschport | `8888` |
| `secret_entry` | Pfad-Präfix des Sicherheitseingangs, z. B. `/opsmini_panel`, leer = deaktiviert | leer |

Nach Konfiguration von `secret_entry` hängen Panel und alle APIs unter dem Präfixpfad (z. B. `http://<host>:8888/opsmini_panel`). In Kombination mit einem Reverse-Proxy lässt sich der tatsächliche Eingang verbergen und Port-Scans sowie Brute-Force-Versuchen vorbeugen.

### database

| Parameter | Beschreibung | Standard |
|------|------|------|
| `path` | Pfad der SQLite-Datendatei | `opsmini.db` |

### jwt

| Parameter | Beschreibung | Standard |
|------|------|------|
| `access_ttl_seconds` | Gültigkeit des Access-Tokens (Sekunden) | `900` |
| `refresh_ttl_seconds` | Gültigkeit des Refresh-Tokens (Sekunden) | `604800` |

> Der JWT-Signaturschlüssel wird hier **nicht mehr** konfiguriert. Beim ersten Start wird automatisch ein 32-Byte-Zufallsschlüssel erzeugt und in der Datenbank persistiert — er landet weder in der Konfigurationsdatei noch wird er in der Oberfläche angezeigt und muss nicht manuell gepflegt werden.

### log

| Parameter | Beschreibung | Standard |
|------|------|------|
| `level` | Log-Level: `info` / `warn` / `error` | `error` |
| `path` | Pfad der Log-Datei; leer = Ausgabe nur nach stdout (systemd journal) | leer |

Das Log-Level steuert die Ausführlichkeit der Ausgabe: `error` gibt nur Fehler aus (für Produktion empfohlen, vermeidet eine Flut von SQL-Abfrage-Logs); `warn` gibt zusätzlich langsame Abfragen und Warnungen aus; `info` gibt alle Logs aus (inkl. SQL-Abfragen, geeignet zur Fehlersuche). Die Ein-Klick-Installation schreibt standardmäßig nach `/data/opsmini/opsmini.log`.

## Optionale Konfigurationsabschnitte

Die folgenden Felder werden in der Konfigurationsstruktur weiterhin unterstützt, sind im offiziellen Beispiel jedoch weggelassen (es gelten die eingebauten Standardwerte) und können bei Bedarf explizit deklariert werden.

### agent (Befehls-Whitelist)

```yaml
agent:
  allowed_commands:        # Befehls-Whitelist-Präfixe der Agent-API
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

| Parameter | Beschreibung | Standard |
|------|------|------|
| `allowed_commands` | Whitelist der Befehlspräfixe, die `/agent/v1/commands` ausführen darf | siehe oben |

> `agent.token` ist veraltet. Das Authentifizierungs-Token der Agent-API wird nun auf der Seite „Einstellungen → API-Token" des Panels erzeugt und verwaltet; leer bedeutet, dass die Agent-API deaktiviert ist.

### metrics (Prometheus-Metriken)

```yaml
metrics:
  enabled: true     # ob der /metrics-Endpunkt aktiviert ist
  user: ""          # Benutzername der Basic-Authentifizierung, leer = keine Authentifizierung
  password: ""      # Passwort der Basic-Authentifizierung
```

| Parameter | Beschreibung | Standard |
|------|------|------|
| `enabled` | ob der `/metrics`-Endpunkt aktiviert ist | `true` |
| `user` | Benutzername der HTTP-Basic-Authentifizierung, leer = keine Authentifizierung | leer |
| `password` | Passwort der HTTP-Basic-Authentifizierung | leer |

> Priorität der Authentifizierungsdaten: Benutzername/Passwort auf der Seite „Einstellungen → Monitoring-Export" des Panels haben **Vorrang** vor `user`/`password` hier. Ein leerer Benutzername bedeutet offenen Zugriff.

## In die Panel-Einstellungen verlagerte Konfiguration

Die folgenden Konfigurationselemente wurden aus `config.yaml` entfernt und werden einheitlich auf der Seite „Einstellungen" des Panels verwaltet (in SQLite gespeichert, zur Laufzeit sofort wirksam):

| Bisherige Konfiguration | Aktueller Verwaltungsort | Beschreibung |
|--------|-----------|------|
| `jwt.secret` | Automatisch erzeugt (keine Verwaltung nötig) | Beim ersten Start wird ein Zufallsschlüssel erzeugt und gespeichert |
| `ai.*` | Einstellungen → AI-LLM-Anbindung | Modell / API-Key / Base-URL / Aktivierungsschalter |
| `agent.token` | Einstellungen → API-Token | Zugriffs-Token der Agent-API, leer = deaktiviert |
| `metrics.user/password` | Einstellungen → Monitoring-Export | Kann die Werte der Konfigurationsdatei zur Laufzeit überschreiben |

Details siehe [Panel-Einstellungen](panel-settings.md).

## Umgebungsvariablen

| Variable | Beschreibung |
|------|------|
| `OPSMINI_AI_KEY` | AI-API-Key mit **höchster Priorität** (vor Panel-Einstellungen und Konfigurationsdatei; vermeidet, dass der Schlüssel auf der Festplatte landet) |

## Befehlszeilenparameter

| Parameter | Beschreibung |
|------|------|
| `-config <path>` | Pfad der Konfigurationsdatei angeben (Ein-Klick-Installation standardmäßig `/data/opsmini/config.yaml`, Betrieb aus dem Quellcode standardmäßig `configs/config.yaml`) |
| `-version` | Versionsinformation ausgeben und beenden |
| `-reset-mfa <username>` | MFA-Bindung des angegebenen Benutzers zurücksetzen (bei verlorenem Code), danach beenden |
| `-reset-pass <username>` | Passwort des angegebenen Benutzers auf ein zufälliges starkes Passwort zurücksetzen und ausgeben, danach beenden |

> `-reset-mfa` / `-reset-pass` bearbeiten direkt die Datenbank, starten keinen Dienst und dienen der Wiederherstellung verlorener Konten.
