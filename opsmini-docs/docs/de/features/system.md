# Systemverwaltung

Die Systemverwaltungsseite bietet eine umfassende Übersicht über die Host-Ressourcen.

## Module

| Modul | Inhalt |
|------|------|
| Systeminformationen | hostname, OS, Kernel-Version, Laufzeit |
| Prozesse | Prozessliste (PID, CPU-, Speicherbelegung) |
| Netzwerk | Netzwerkkarten und Verkehr |
| Ports | Lauschstatus der Ports |
| Festplatte | Festplatten und Einhängepunkte, Kapazitätsauslastung |
| Systembenutzer | Liste der Systembenutzer |
| Benutzergruppen | Liste der Benutzergruppen |
| Firewall | Firewall-Status und -Regeln |

## Metrik-Quelle

Die Systemressourcen-Informationen werden von `gopsutil` erfasst und sind plattformübergreifend kompatibel mit Linux / macOS (macOS nur für Entwicklung & Debugging).
