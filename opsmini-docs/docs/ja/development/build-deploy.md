# ビルドとデプロイ

## 前提となる依存関係

- Go 1.25+（純 Go ドライバで CGO なし、直接クロスコンパイル可能）

## コードの取得

```bash
git clone <リポジトリのアドレス> opsmini
cd opsmini
export GOPROXY=https://goproxy.cn,direct   # 国内向け高速化
go mod tidy
```

## ビルドコマンド

```bash
make build          # 現在のプラットフォームをコンパイル → dist/opsmini
make build-all      # linux/darwin amd64/arm64 をクロスコンパイル
make clean          # dist/ をクリーンアップ
make version        # バージョン情報を表示
```

## バージョン番号の注入

ビルド情報は `-ldflags -X main.version/-X main.buildTime/-X main.gitCommit` で注入します：

```bash
make build VERSION=v1.0.0
./dist/opsmini -version
# opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

## 成果物の命名

`opsmini-<version>-<os>-<arch>`（例：`opsmini-v1.0.0-linux-amd64`）。

## 配布成果物

| 成果物 | 説明 |
|------|------|
| `opsmini` 単一バイナリ | バックエンド API + フロントエンド UI + SQLite。約 28~30MB の静的リンク |
| `configs/config.yaml` | 設定テンプレート |
| `opsmini.db` | 初回実行時に自動生成 |

> デプロイはバイナリ + 設定ファイルのコピーだけで、その他のランタイム依存はありません。
