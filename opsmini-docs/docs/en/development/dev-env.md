# Local Development Environment

## Install Go 1.25+

```bash
# macOS（Apple Silicon）
curl -sL -o /tmp/go.tar.gz https://go.dev/dl/go1.25.0.darwin-arm64.tar.gz
mkdir -p ~/go-sdk && tar -C ~/go-sdk -xzf /tmp/go.tar.gz
export PATH=~/go-sdk/go/bin:$PATH

# Linux x64
curl -sL -o /tmp/go.tar.gz https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf /tmp/go.tar.gz
export PATH=/usr/local/go/bin:$PATH
```

## Run the Panel

```bash
go run ./cmd/agent -config configs/config.yaml
# or build first, then run
make build && ./dist/opsmini -config configs/config.yaml
```

Visit `http://localhost:8888`, default account `opsmini`, password in the startup log.

## Frontend Notes

- The frontend is a single file `web/index.html` (Vue 3 inline SPA) + `web/static/` static libraries
- It is embedded into the binary via `go:embed` in `web/embed.go`
- After modifying the frontend, run `make build` again for changes to take effect

## FAQ

- `go mod tidy` gets stuck → configure `GOPROXY=https://goproxy.cn,direct`
- Build reports a `vendor` directory conflict → the frontend dependency directory has been renamed to `static/`, do not create a `vendor/` directory again
