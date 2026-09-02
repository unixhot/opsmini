# 本地開發環境

## 安裝 Go 1.25+

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

## 執行面板

```bash
go run ./cmd/agent -config configs/config.yaml
# 或构建后运行
make build && ./dist/opsmini -config configs/config.yaml
```

訪問 `http://localhost:8888`，預設賬號 `opsmini`，密碼見啟動日誌。

## 前端說明

- 前端為單檔案 `web/index.html`（Vue 3 內聯 SPA）+ `web/static/` 靜態庫
- 透過 `web/embed.go` 的 `go:embed` 嵌入二進位制
- 修改前端後重新 `make build` 生效

## 常見問題

- `go mod tidy` 卡住 → 配置 `GOPROXY=https://goproxy.cn,direct`
- 構建報 `vendor` 目錄衝突 → 前端依賴目錄已改名 `static/`，勿再建 `vendor/`
