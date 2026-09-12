# Systemanforderungen

## Ziel-Host

| Punkt | Mindestanforderung | Empfohlen |
|----|----------|------|
| Betriebssystem | Gängige Linux-Distributionen (x86_64 / aarch64) | Ubuntu 20.04+ / Debian 11+ / CentOS 7+ |
| CPU | 1 Kern | 2 Kerne |
| Arbeitsspeicher | 512 MB | 1 GB+ |
| Festplatte | 200 MB (Binary + Daten) | 1 GB+ (inkl. Logs und SQLite) |
| Netzwerk | Zugriff auf das Internet (beim ersten Download) | Stabiles Intranet genügt |

## Architektur-Unterstützung

| Plattform | Verwendung |
|------|------|
| `linux/amd64` | Gängige x86_64-Server (Produktion) |
| `linux/arm64` | ARM-Server (Graviton / Raspberry Pi / 鲲鹏 / 飞腾, Produktion) |
| `darwin/amd64` / `darwin/arm64` | macOS-Entwicklungsrechner (nur Entwicklung & Debugging, keine Auslieferung) |

## Laufzeitabhängigkeiten

- **Null Laufzeitabhängigkeiten**: Frontend, SQLite und statische Ressourcen sind vollständig in das Binary eingebettet; keine Installation von PHP / Node / Datenbank erforderlich
- Für die Docker-Verwaltung muss Docker auf dem Host installiert sein (das Panel selbst hängt nicht von Docker ab)

## Berechtigungen

- Das Ein-Klick-Installationsskript benötigt `root` oder `sudo`
- Der Panel-Prozess sollte unter einem dedizierten Benutzer mit geringen Rechten (z. B. `opsmini`) laufen — siehe [systemd-Verwaltung](systemd.md)
