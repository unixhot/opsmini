# Prometheus-Metriken (/metrics)

OpsMini enthält einen **node_exporter-kompatiblen** `/metrics`-Endpunkt; ohne zusätzliche node_exporter-Installation kann Prometheus direkt scrapen.

## Aktivierung & Authentifizierung

In `config.yaml` aktivieren (standardmäßig bereits aktiviert):

```yaml
metrics:
  enabled: true      # ob der /metrics-Endpunkt aktiviert ist
  user: ""           # Benutzername der Basic-Authentifizierung, leer = keine Authentifizierung
  password: ""       # Passwort der Basic-Authentifizierung
```

Priorität der Authentifizierungsdaten: Benutzername/Passwort auf der Seite „Einstellungen → Monitoring-Export" des Panels haben **Vorrang** vor der Konfigurationsdatei; ein leerer Benutzername bedeutet keine Authentifizierung.

## Prometheus-Konfiguration

```yaml
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
    basic_auth:            # falls Authentifizierung aktiviert ist, die entsprechenden Zugangsdaten konfigurieren
      username: 'monitor'
      password: '<password>'
```

## Metrik-Familien

Die Kernmetriken sind an node_exporter angeglichen; Community-Node-Dashboards lassen sich direkt übernehmen:

| Metrik-Familie | Beschreibung |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | Kumulierte Sekunden pro Kern und Modus |
| `node_memory_MemTotal_bytes` u. a. | Speicher Total / Free / Available / Buffers / Cached |
| `node_filesystem_size_bytes{mountpoint}` | Kapazität / Verfügbar / Auslastung je Einhängepunkt |
| `node_network_receive_bytes_total{device}` | Sende-/Empfangsverkehr je Netzwerkkarte |
| `node_load1` / `node_load5` / `node_load15` | Last |
| `node_uname_info` / `node_boot_time_seconds` | Host-Informationen und Startzeit |

## Hinweise

- Die Implementierung verwendet die bereits von `gopsutil` erfassten Daten wieder und gibt sie im Prometheus-Textformat aus
- Beibehaltung der Einzel-Binary-Auslieferung, ohne Einbettung eines node_exporter-Prozesses
- Bei konfiguriertem `server.secret_entry` hängt der Endpunkt ebenfalls unter dem Präfix (z. B. `/opsmini_panel/metrics`)
