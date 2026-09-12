# ローカル開発環境

## Go 1.25+ のインストール

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

## パネルの実行

```bash
go run ./cmd/agent -config configs/config.yaml
# またはビルド後に実行
make build && ./dist/opsmini -config configs/config.yaml
```

`http://localhost:8888` にアクセスします。デフォルトアカウントは `opsmini`、パスワードは起動ログを参照してください。

## フロントエンドの説明

- フロントエンドは単一ファイル `web/index.html`（Vue 3 インライン SPA）+ `web/static/` 静的ライブラリです
- `web/embed.go` の `go:embed` でバイナリに埋め込みます
- フロントエンドを変更したら `make build` を再実行すると反映されます

## よくある問題

- `go mod tidy` が進まない → `GOPROXY=https://goproxy.cn,direct` を設定
- ビルドで `vendor` ディレクトリの競合エラー → フロントエンドの依存ディレクトリは `static/` に改名済みです。再度 `vendor/` を作成しないでください
