# 本地开发环境

## 安装 Go 1.25+

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

## 运行面板

```bash
go run ./cmd/agent -config configs/config.yaml
# 或构建后运行
make build && ./dist/opsmini -config configs/config.yaml
```

访问 `http://localhost:8888`，默认账号 `opsmini`，密码见启动日志。

## 前端说明

- 前端为单文件 `web/index.html`（Vue 3 内联 SPA）+ `web/static/` 静态库
- 通过 `web/embed.go` 的 `go:embed` 嵌入二进制
- 修改前端后重新 `make build` 生效

## 常见问题

- `go mod tidy` 卡住 → 配置 `GOPROXY=https://goproxy.cn,direct`
- 构建报 `vendor` 目录冲突 → 前端依赖目录已改名 `static/`，勿再建 `vendor/`
