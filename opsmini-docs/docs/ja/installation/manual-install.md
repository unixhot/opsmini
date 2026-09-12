# 手動インストール（ソースコードからビルド）

ワンクリックスクリプトを使いたくない場合は、ソースコードからビルドして手動でデプロイできます。

## 前提となる依存関係

| 依存 | バージョン要件 | 説明 |
|------|----------|------|
| Go | **1.25+** | 純 Go ドライバで CGO なし、任意のプラットフォームで直接クロスコンパイル可能 |
| メモリ | ≥ 512MB | ビルド・実行ともに軽量 |

> **なぜ CGO ツールチェーンが不要か**：SQLite ドライバは `github.com/glebarez/sqlite`（純 Go 実装）を採用しており、
> `CGO_ENABLED=0` で任意のプラットフォーム上で**静的リンク**のバイナリをクロスコンパイルできます。

## コードと依存関係の取得

```bash
git clone <リポジトリのアドレス> opsmini
cd opsmini

# 中国国内のネットワークでは goproxy ミラーを設定
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

## ビルド

```bash
# 現在のプラットフォームをコンパイル → dist/opsmini
make build

# 全ターゲットプラットフォームをクロスコンパイル
make build-all
# 成果物：
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64

# バージョン番号を明示的に指定
make build VERSION=v1.0.0

# バイナリのバージョンを確認
./dist/opsmini -version
```

> 正式リリースの手順：`git tag v1.0.0 && make build-all`。`VERSION` は自動でタグ名になります。

## 成果物アーキテクチャの検証

```bash
file dist/*
# 期待値：ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64、すべて静的リンク
```

## 手動デプロイ

### ディレクトリ計画

```
/data/opsmini/
├── opsmini              # バイナリ
├── config.yaml          # 設定ファイル
└── opsmini.db           # データ（初回起動時に自動生成）
```

### バイナリと設定の配置

```bash
sudo mkdir -p /data/opsmini
sudo cp dist/opsmini-*-linux-amd64 /data/opsmini/opsmini
sudo cp configs/config.yaml /data/opsmini/config.yaml

# 設定変更：データベースの絶対パス（JWT 署名シークレットの設定は不要。初回起動時に自動生成）
sudo sed -i 's|path: "opsmini.db"|path: "/data/opsmini/opsmini.db"|' /data/opsmini/config.yaml

sudo chmod +x /data/opsmini/opsmini
```

### 実行

```bash
/data/opsmini/opsmini -config /data/opsmini/config.yaml
```

初回起動時に自動でテーブルを作成し、デフォルトの管理者アカウントを書き込み、ログにアカウントとパスワードを表示します。`http://<host>:8888` にアクセスするとパネルが表示されます。

## 次のステップ

- [systemd 管理](systemd.md)
- [設定の詳細](../configuration/config-file.md)
