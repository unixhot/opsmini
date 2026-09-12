# Lokale Entwicklungsumgebung

## Go 1.25+ installieren

```bash
# macOS (Apple Silicon)
curl -sL -o /tmp/go.tar.gz https://go.dev/dl/go1.25.0.darwin-arm64.tar.gz
mkdir -p ~/go-sdk && tar -C ~/go-sdk -xzf /tmp/go.tar.gz
export PATH=~/go-sdk/go/bin:$PATH

# Linux x64
curl -sL -o /tmp/go.tar.gz https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf /tmp/go.tar.gz
export PATH=/usr/local/go/bin:$PATH
```

## Panel ausführen

```bash
go run ./cmd/agent -config configs/config.yaml
# oder nach dem Build ausführen
make build && ./dist/opsmini -config configs/config.yaml
```

`http://localhost:8888` aufrufen; Standardkonto `opsmini`, Passwort siehe Startprotokoll.

## Hinweise zum Frontend

- Das Frontend besteht aus der Einzeldatei `web/index.html` (Vue-3-Inline-SPA) + den statischen Bibliotheken unter `web/static/`
- Es wird über `go:embed` in `web/embed.go` in das Binary eingebettet
- Nach Änderungen am Frontend erneut `make build` ausführen, damit sie wirksam werden

## Häufige Probleme

- `go mod tidy` hängt → `GOPROXY=https://goproxy.cn,direct` konfigurieren
- Build meldet einen Konflikt im `vendor`-Verzeichnis → das Frontend-Abhängigkeitsverzeichnis wurde bereits in `static/` umbenannt; kein `vendor/`-Verzeichnis erneut anlegen
