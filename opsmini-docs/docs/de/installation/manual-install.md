# Manuelle Installation (Build aus dem Quellcode)

Wenn du das Ein-Klick-Skript nicht verwenden möchtest, kannst du aus dem Quellcode bauen und manuell deployen.

## Voraussetzungen

| Abhängigkeit | Versionsanforderung | Beschreibung |
|------|----------|------|
| Go | **1.25+** | Reines Go ohne CGO, plattformübergreifend direkt cross-kompilierbar |
| Arbeitsspeicher | ≥ 512 MB | Build und Laufzeit sind beide leichtgewichtig |

> **Warum keine CGO-Toolchain nötig ist**: Der SQLite-Treiber verwendet `github.com/glebarez/sqlite` (reine Go-Implementierung).
> Mit `CGO_ENABLED=0` lässt sich auf jeder Plattform ein **statisch gelinktes** Binary cross-kompilieren.

## Code und Abhängigkeiten beziehen

```bash
git clone <Repository-Adresse> opsmini
cd opsmini

# In China muss ein goproxy-Spiegel konfiguriert werden
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

## Build

```bash
# Für die aktuelle Plattform kompilieren → dist/opsmini
make build

# Cross-Kompilierung für alle Zielplattformen
make build-all
# Ergebnis:
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64

# Versionsnummer explizit angeben
make build VERSION=v1.0.0

# Binary-Version anzeigen
./dist/opsmini -version
```

> Offizieller Release-Prozess: `git tag v1.0.0 && make build-all`, wobei `VERSION` automatisch den Tag-Namen übernimmt.

## Architektur des Ergebnisses prüfen

```bash
file dist/*
# Erwartet: ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64, alle statisch gelinkt
```

## Manuelles Deployment

### Verzeichnisplanung

```
/data/opsmini/
├── opsmini              # Binary
├── config.yaml          # Konfigurationsdatei
└── opsmini.db           # Daten (werden beim ersten Start automatisch erzeugt)
```

### Binary und Konfiguration ablegen

```bash
sudo mkdir -p /data/opsmini
sudo cp dist/opsmini-*-linux-amd64 /data/opsmini/opsmini
sudo cp configs/config.yaml /data/opsmini/config.yaml

# Konfiguration ändern: absoluter Pfad der Datenbank (JWT-Signaturschlüssel muss nicht konfiguriert werden, wird beim ersten Start automatisch erzeugt)
sudo sed -i 's|path: "opsmini.db"|path: "/data/opsmini/opsmini.db"|' /data/opsmini/config.yaml

sudo chmod +x /data/opsmini/opsmini
```

### Ausführen

```bash
/data/opsmini/opsmini -config /data/opsmini/config.yaml
```

Beim ersten Start werden die Tabellen automatisch angelegt und das Standard-Administratorkonto geschrieben; das Protokoll gibt Konto und Passwort aus. Öffne `http://<host>:8888`, um das Panel zu sehen.

## Nächste Schritte

- [systemd-Verwaltung](systemd.md)
- [Konfiguration im Detail](../configuration/config-file.md)
