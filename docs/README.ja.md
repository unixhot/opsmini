<div align="center">

# OpsMini

**サーバー上で動く AI 運用エージェント**

[English](README.en.md) · [简体中文](../README.md) · [繁體中文](README.zh-TW.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [ไทย](README.th.md) · [Deutsch](README.de.md)

</div>

---

## 日本語

OpsMini は Linux ホストに常駐する AI 運用エージェントです。指標を集め、異常を判断し、自分で対処します。危険な操作の前には必ず止まって確認を求めます。Web UI は観測窓にすぎず、何もクリックしなくても毎日の作業を終えてレポートを残します。数十 MB の静的バイナリ 1 つで、データはすべて端末内に残ります。

### 機能

- **AI 運用支援** — 自然言語による診断、ログ分析、コマンド実行
- **標準 REST API** — `/api/v1`（ブラウザ UI）と `/agent/v1`（マシン間）
- **Docker 管理** — コンテナ、イメージ、ボリューム、ネットワーク
- **Web ターミナル** — WebSocket + pty による SSH ライクな対話型シェル
- **7 言語 i18n** — 簡体/繁体中国語、英語、日本語、韓国語、タイ語、ドイツ語
- **単一バイナリ** — フロントエンドを `go:embed` で埋め込み、ランタイム依存ゼロ

### 技術スタック

Go · Gin · GORM · SQLite（純 Go）· Vue 3 · ECharts

### クイックスタート

```bash
make build          # 現在のプラットフォーム向けにビルド
make build-all      # Linux amd64/arm64 向けにクロスコンパイル

./dist/opsmini -config configs/config.yaml
# http://localhost:8888 を開く（デフォルト: opsmini、パスワードは起動ログ）
```

### ドキュメント

- [開発者ガイド](docs/developer-guide.md)
- [バックエンドアーキテクチャ](docs/backend-architecture.md)
- [ビルドとデプロイ](docs/build-and-deploy.md)
- [ホストセキュリティ設計](host-security-design.md)
- [セキュリティ監査レポート](security-audit.md)

### 著作権

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
