<div align="center">

# OpsMini

**AI 駆動のサーバー管理パネル**

[English](../README.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [ไทย](README.th.md) · [Deutsch](README.de.md)

</div>

---

## 日本語

OpsMini は AI 駆動のサーバー管理パネル（BaoTa / 1Panel に相当）で、内蔵の AI 運用支援と外部統合のための標準 REST API が特長です。

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
- [ホストセキュリティ設計](docs/security-audit.md)

### 著作権

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
