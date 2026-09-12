---
title: OpsMini — 軽量 Linux ホスト運用管理パネル
hide:
  - navigation
  - toc
---

<div class="home-hero" markdown>

# OpsMini

**軽量 Linux ホスト運用管理パネル** — 内蔵 AI 大規模モデル運用、標準 REST API による外部連携

宝塔 / 1Panel に匹敵し、単一ホストの運用をよりシンプルに、よりスマートに、より連携しやすく。

<div class="home-cta" markdown>
[クイックインストール](getting-started/quick-install.md){ .md-button .md-button--primary }
[ドキュメントを見る](getting-started/index.md){ .md-button }
[:fontawesome-brands-github: GitHub](https://github.com/unixhot/opsmini){ .md-button }
</div>

</div>

---

## 二つの主要な差別化ポイント

<div class="grid cards" markdown>

-   :material-robot-outline: **AI 大規模モデル運用**

    ---

    自然言語で障害を診断、ログを分析、運用コマンドを実行。OpenAI / DeepSeek / Qwen / Ollama に接続し、AI を運用の副操縦士に。

-   :material-api: **標準 REST API**

    ---

    内蔵の `/api/v1`（パネル API）と `/agent/v1`（マシン対マシン API）により、あらゆる監視プラットフォーム、自動化スクリプト、オーケストレーションツールが直接連携・呼び出し可能。

</div>

---

## 機能一覧

<div class="grid cards" markdown>

-   :material-view-dashboard-outline: **ダッシュボードと監視**

    ---

    リアルタイムの CPU / メモリ / ディスク / ネットワーク時系列、アラートルールとアラートイベントのクローズドループ。

-   :material-docker: **コンテナ管理**

    ---

    Docker のコンテナ・イメージ・ボリューム・ネットワークの 4 種リソースを完全管理。さらにアプリストアでよく使うアプリをワンクリックインストール。

-   :material-console: **Web ターミナル**

    ---

    WebSocket + pty で実装した SSH ライクな対話型ターミナル。ブラウザから直接サーバーを操作。

-   :material-shield-check-outline: **ホストセキュリティ**

    ---

    ベースライン検査、ファイル整合性監視（FIM）、脅威検出、ファイアウォール、ログインセキュリティ。セキュリティ状況をひとつの画面で把握。

-   :material-folder-outline: **ファイルとウェブサイト**

    ---

    ファイルの閲覧 / アップロード / 編集、Nginx ウェブサイト、データベース、SSL 証明書のワンストップ管理。

-   :material-translate: **7 言語・単一バイナリ**

    ---

    簡体字/繁体字中国語、英語、日本語、韓国語、タイ語、ドイツ語の 7 言語 UI。フロントエンドは `go:embed` で埋め込み、約 30MB の静的リンク、ゼロのランタイム依存。

</div>

---

## ワンクリックインストール

<div class="home-section" markdown>

### 5 分でスタート

```bash
# Linux x86_64 / aarch64、ワンクリックスクリプトで /data/opsmini にインストール、ポート 8888
curl -fsSL https://opsmini.com/install.sh | sudo bash
```

初回起動時に管理者アカウント `opsmini` とランダムパスワードを自動生成（起動ログ参照）、`http://<host>:8888` を開くだけです。

[完全なインストールドキュメントを見る →](installation/index.md){ .md-button }

</div>

---

<div class="home-section" markdown>

## 今すぐ始める

ワンコマンドでデプロイ、AI が運用を支援、標準 API で連携を実現。

[インストール開始](getting-started/quick-install.md){ .md-button .md-button--primary }
[ドキュメントを探す](getting-started/index.md){ .md-button }

</div>
