# よくある質問

## インストールと起動

**Q：`go mod tidy` が進まない、または Bad Gateway が出る？**

中国国内のネットワークでは `proxy.golang.org` がブロックされる可能性があります。`export GOPROXY=https://goproxy.cn,direct` を実行してください。

**Q：Go のバージョンが低すぎると表示される？**

OpsMini は Go 1.25+ が必要です。公式のプリコンパイル済みバイナリをダウンロードして PATH に追加してください。

**Q：`/` にアクセスすると 301 または空白になる？**

フロントエンドの静的リソースは `go:embed` で埋め込まれています。`make build` の成果物を使用し、フロントエンドのディレクトリを手動で分割しないでください。

## アカウントとセキュリティ

**Q：管理者パスワードを忘れた？**

サーバー上で `opsmini -config <path> -reset-pass opsmini` を実行してランダムパスワードにリセットします（詳しくは [インストールとデプロイ](../installation/index.md)）。

**Q：MFA 二要素認証コードを紛失した？**

`opsmini -config <path> -reset-mfa opsmini` を実行して MFA バインディングを解除し、再ログインしてバインドし直します。

## デプロイと運用

**Q：バージョンをアップグレードするには？**

`opsmini.db` をバックアップ → バイナリを置き換え → サービスを再起動。SQLite は GORM が自動マイグレーションするため、通常は手動でテーブルを変更する必要はありません。

**Q：リバースプロキシ経由で Web ターミナルに接続できない？**

Nginx で WebSocket のアップグレードヘッダーを有効にする必要があります（`proxy_http_version 1.1` + `Upgrade`/`Connection`）。詳しくは [ポートとリバースプロキシ](../configuration/ports-proxy.md)。

**Q：Prometheus にローカルマシンを監視させるには？**

設定で `metrics` を有効にすると、Prometheus が直接 `http://<host>:8888/metrics` をスクレイプします。
