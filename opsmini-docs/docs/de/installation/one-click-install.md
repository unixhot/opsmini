# Ein-Klick-Skript-Installation

Das offizielle `install.sh` schließt das Deployment auf einem Linux-Host mit einem Klick ab.

## Schnellstart

```bash
# Ein-Klick-Installation (empfohlen; das Skript lädt das Binary automatisch architekturabhängig aus dem Alibaba Cloud OSS herunter)
curl -fsSL https://opsmini.com/install.sh | sudo bash

# Installation mit lokalem Binary (Standard: Installation nach /data/opsmini, Port 8888)
sudo ./install.sh -b ./opsmini

# Benutzerdefinierte Download-Adresse (unterstützt nacktes Binary oder .tar.gz)
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# Benutzerdefiniertes Verzeichnis und Port
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## Parameter

| Parameter | Beschreibung | Standardwert |
|------|------|--------|
| `-d, --dir <path>` | Installationsverzeichnis | `/data/opsmini` |
| `-p, --port <port>` | Lauschport des Panels | `8888` |
| `-b, --binary <path>` | Pfad zum opsmini-Binary | Wenn nicht angegeben, Download aus OSS |
| `-u, --url <url>` | Download von URL (nacktes Binary oder `.tar.gz`) | Offizielle OSS-Adresse |
| `-n, --no-systemd` | Keinen systemd-Dienst registrieren | — |
| `-h, --help` | Hilfe | — |

## Installationsverhalten

1. Installationsverzeichnis anlegen (Standard `/data/opsmini`)
2. Binary nach `<Verzeichnis>/opsmini` herunterladen / kopieren (Standard: Download architekturabhängig aus Alibaba Cloud OSS)
3. `<Verzeichnis>/config.yaml` erzeugen (Port, SQLite-Pfad, JWT-Gültigkeit, Log-Level und Log-Datei; JWT-Schlüssel und Agent-Token werden beim ersten Start automatisch generiert und in der Datenbank gespeichert)
4. Ein 16-stelliges zufälliges Administratorpasswort erzeugen und nach `<Verzeichnis>/.init_passwd` schreiben (Berechtigung 600)
   - **Neuinstallation**: beim ersten Start über die Umgebungsvariable `OPSMINI_INIT_PASSWORD` injizieren
   - **Neuinstallation** (Datenbank bereits vorhanden): Passwort automatisch mit `-reset-pass` zurücksetzen; das ausgegebene Passwort ist das gültige Passwort
5. Den systemd-Dienst `opsmini.service` registrieren und starten
6. Die Antwort des Dienstes prüfen und Zugriffsadresse / Benutzername / Passwort / Pfad der Log-Datei ausgeben

## Ergebnis

```
/data/opsmini/
├── opsmini              # Binary
├── config.yaml          # Konfiguration (600)
├── opsmini.db           # SQLite-Datenbank (wird nach dem ersten Start erzeugt)
├── opsmini.log          # Laufzeit-Log-Datei
└── .init_passwd         # Initiales Passwort (600)
```

## Binary-Beschaffung

- **Offizielle Distribution (Alibaba Cloud OSS)**: `https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v<Version>-linux-<Architektur>`, von dort lädt das Skript standardmäßig herunter
- **Lokale Entwicklung**: `make build-all` im Projektstammverzeichnis erzeugt `dist/opsmini-<ver>-linux-amd64` / `-arm64`; das Skript sucht automatisch nach der Architektur

## Kompatibilität

- Nur `x86_64` / `aarch64` werden unterstützt
- In Umgebungen ohne systemd mit `-n` die Dienstregistrierung überspringen und stattdessen manuell starten

## Konto-Wiederherstellung {: #account-recovery }

Bei vergessenem Passwort oder verlorenem MFA-Code auf dem Server ausführen (zuerst den Dienst stoppen, nach Abschluss wieder starten):

```bash
# Passwort zurücksetzen
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-pass opsmini
systemctl start opsmini

# MFA-Bindung entfernen
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-mfa opsmini
systemctl start opsmini
```

> Hinweis: `-reset-mfa` / `-reset-pass` sind Unterbefehle zur Konto-Wiederherstellung. Sie bearbeiten direkt die SQLite-Datenbank und beenden sich danach, ohne den Webdienst zu starten. Stelle unbedingt sicher, dass der Dienst während der Ausführung gestoppt ist.
