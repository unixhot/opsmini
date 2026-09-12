# Build & Deployment

## Voraussetzungen

- Go 1.25+ (reiner Go-Treiber ohne CGO, direkt cross-kompilierbar)

## Code beziehen

```bash
git clone <Repository-Adresse> opsmini
cd opsmini
export GOPROXY=https://goproxy.cn,direct   # Beschleunigung in China
go mod tidy
```

## Build-Befehle

```bash
make build          # für die aktuelle Plattform kompilieren → dist/opsmini
make build-all      # cross-kompilieren für linux/darwin amd64/arm64
make clean          # dist/ bereinigen
make version        # Versionsinformation ausgeben
```

## Versionsnummer-Injektion

Build-Informationen werden über `-ldflags -X main.version/-X main.buildTime/-X main.gitCommit` injiziert:

```bash
make build VERSION=v1.0.0
./dist/opsmini -version
# opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

## Benennung der Ergebnisse

`opsmini-<version>-<os>-<arch>`, z. B. `opsmini-v1.0.0-linux-amd64`.

## Auslieferungsartefakte

| Artefakt | Beschreibung |
|------|------|
| `opsmini`-Einzelbinary | Backend-API + Frontend-UI + SQLite, ca. 28–30 MB statisch gelinkt |
| `configs/config.yaml` | Konfigurationsvorlage |
| `opsmini.db` | wird beim ersten Lauf automatisch erzeugt |

> Deployment bedeutet lediglich das Kopieren von Binary + Konfigurationsdatei, ohne weitere Laufzeitabhängigkeiten.
