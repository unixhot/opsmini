# Dashboard & Monitoring

OpsMini enthält integrierte Metrik-Erfassung und Monitoring-Alarmfähigkeiten, ohne dass zusätzliche Monitoring-Komponenten installiert werden müssen.

## Dashboard

Nach der Anmeldung gelangst du standardmäßig zum Dashboard, das Folgendes anzeigt:

- **Ressourcenüberblick**: CPU-Auslastung, Speicherbelegung, Last, Festplatte, Netzwerkverkehr
- **Echtzeit-Zeitreihen**: Kurven von CPU / Speicher / Festplatte / Netzwerk über die Zeit (gerendert mit ECharts)
- **Systemstatus**: Laufzeit, Host-Informationen

Der Metrik-Kollektor sampelt alle 10 Sekunden, ein Ringpuffer im Speicher bewahrt 24 Stunden auf; historische Daten werden nach Downsampling in SQLite gespeichert (1-Minuten-Granularität, 7 Tage Aufbewahrung).

## Monitoring

Die Monitoring-Seite bietet:

- **Zeitreihenabfrage**: historische Metriken nach Zeitbereich ansehen
- **Alarmregeln**: benutzerdefinierte Alarm-Schwellenwerte und -Dauer für Metriken wie CPU / Speicher / Festplatte / Dienst
- **Alarmereignisse**: ausgelöste Alarmereignisse und deren Status ansehen

### Beispiel für Alarmregeln

| Feld | Beschreibung |
|------|------|
| Name | Name der Regel, z. B. „CPU dauerhaft zu hoch" |
| Metrik | `cpu` / `mem` / `disk` / `service` |
| Bedingung | z. B. `>90%` |
| Dauer | Zeit, während der die Bedingung vor dem Auslösen erfüllt sein muss |
| Benachrichtigungsart | zugeordneter Benachrichtigungskanal |

## Benachrichtigungen

Nach Auslösen eines Alarms kann über das Benachrichtigungscenter an Administratoren zugestellt werden; unterstützt werden In-Panel-Benachrichtigungen und Gelesen-Verwaltung.

## Prometheus-Integration

Das Panel enthält einen node_exporter-kompatiblen `/metrics`-Endpunkt, den Prometheus direkt scrapen kann. Details siehe [Prometheus-Metriken](../api/prometheus.md).
