# Schnellinstallation in 5 Minuten

Verwende das offizielle Installationsskript `install.sh`, um das Deployment auf einem Linux-Host mit einem Klick abzuschließen.

## Voraussetzungen

- Ziel-Host: Linux `x86_64` oder `aarch64`
- `sudo`-Berechtigung
- Zugriff auf das Internet (zum Herunterladen des Binaries) oder ein im Voraus bereitgestelltes Binary

## Ein-Klick-Installation

```bash
# Ein-Klick-Installation (empfohlen; das Skript lädt das Binary automatisch architekturabhängig aus dem Alibaba Cloud OSS herunter)
curl -fsSL https://opsmini.com/install.sh | sudo bash

# Installation mit lokalem Binary (Standard: Installation nach /data/opsmini, Port 8888)
sudo ./install.sh -b ./opsmini

# Benutzerdefinierte Download-Adresse
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# Benutzerdefiniertes Verzeichnis und Port
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## Installationsparameter

| Parameter | Beschreibung | Standardwert |
|------|------|--------|
| `-d, --dir <path>` | Installationsverzeichnis | `/data/opsmini` |
| `-p, --port <port>` | Lauschport des Panels | `8888` |
| `-b, --binary <path>` | Pfad zum opsmini-Binary | Wenn nicht angegeben, Download aus OSS |
| `-u, --url <url>` | Download von URL (`.tar.gz` oder nacktes Binary) | Offizielle OSS-Adresse |
| `-n, --no-systemd` | Keinen systemd-Dienst registrieren | — |

## Installationsverhalten

1. Installationsverzeichnis anlegen (Standard `/data/opsmini`)
2. Binary nach `<Verzeichnis>/opsmini` herunterladen / kopieren (Standard: Download architekturabhängig aus Alibaba Cloud OSS)
3. `<Verzeichnis>/config.yaml` erzeugen (Port, SQLite-Pfad, JWT-Gültigkeit, Log-Level und Log-Datei; JWT-Schlüssel und Agent-Token werden beim ersten Start automatisch generiert und in der Datenbank gespeichert)
4. Ein 16-stelliges zufälliges Administratorpasswort erzeugen und nach `<Verzeichnis>/.init_passwd` schreiben (Berechtigung 600)
5. Den systemd-Dienst `opsmini.service` registrieren und starten
6. Die Antwort des Dienstes prüfen und Zugriffsadresse / Benutzername / Passwort / Pfad der Log-Datei ausgeben

## Ergebnisstruktur

```
/data/opsmini/
├── opsmini              # Binary
├── config.yaml          # Konfiguration (600)
├── opsmini.db           # SQLite-Datenbank (wird nach dem ersten Start erzeugt)
├── opsmini.log          # Laufzeit-Log-Datei
└── .init_passwd         # Initiales Passwort (600)
```

## Erste Anmeldung

- Aufruf von `http://<host>:8888`
- Benutzername: `opsmini`
- Passwort: das vom Installationsskript ausgegebene Passwort oder `/data/opsmini/.init_passwd` einsehen

> ⚠️ Notiere das Passwort sofort und ändere es nach der Anmeldung. Bei einer Neuinstallation wird das Passwort automatisch zurückgesetzt und erneut ausgegeben.

## Nächste Schritte

- [Erste Anmeldung und Initialisierung](first-steps.md)
- [Manuelles Deployment (systemd)](../installation/manual-install.md)
- [Reverse-Proxy und HTTPS](../configuration/ports-proxy.md)
