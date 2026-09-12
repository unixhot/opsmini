# 5 分でクイックインストール

公式インストールスクリプト `install.sh` を使用して、Linux ホストにワンクリックでデプロイします。

## 前提

- 対象ホスト：Linux `x86_64` または `aarch64`
- `sudo` 権限を保持
- 外部ネットワークにアクセス可能（バイナリのダウンロード用）、またはバイナリを事前に用意

## ワンクリックインストール

```bash
# ワンクリックインストール（推奨。スクリプトがアーキテクチャに応じて Alibaba Cloud OSS からバイナリを自動ダウンロード）
curl -fsSL https://opsmini.com/install.sh | sudo bash

# ローカルバイナリを指定してインストール（デフォルトで /data/opsmini にインストール、ポート 8888）
sudo ./install.sh -b ./opsmini

# ダウンロード先をカスタマイズ
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# ディレクトリとポートをカスタマイズ
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## インストールパラメータ

| パラメータ | 説明 | デフォルト値 |
|------|------|--------|
| `-d, --dir <path>` | インストールディレクトリ | `/data/opsmini` |
| `-p, --port <port>` | パネルのリッスンポート | `8888` |
| `-b, --binary <path>` | opsmini バイナリのパス | 未指定時は OSS からダウンロード |
| `-u, --url <url>` | URL からダウンロード（`.tar.gz` または生のバイナリ） | OSS 公式アドレス |
| `-n, --no-systemd` | systemd サービスを登録しない | — |

## インストール時の動作

1. インストールディレクトリを作成（デフォルト `/data/opsmini`）
2. バイナリをダウンロード / コピーして `<ディレクトリ>/opsmini` に配置（デフォルトでアーキテクチャに応じて Alibaba Cloud OSS からダウンロード）
3. `<ディレクトリ>/config.yaml` を生成（ポート、SQLite パス、JWT 有効期間、ログレベルとログファイル。JWT シークレットと Agent Token は初回起動時に自動生成して DB に保存）
4. 16 桁のランダム管理者パスワードを生成し、`<ディレクトリ>/.init_passwd`（権限 600）に書き込み
5. systemd サービス `opsmini.service` を登録して起動
6. サービスの応答をプローブし、アクセスアドレス / ユーザー名 / パスワード / ログファイルのパスを表示

## 成果物の構成

```
/data/opsmini/
├── opsmini              # バイナリ
├── config.yaml          # 設定（600）
├── opsmini.db           # SQLite データベース（初回起動後に生成）
├── opsmini.log          # 実行ログファイル
└── .init_passwd         # 初期パスワード（600）
```

## 初回ログイン

- `http://<host>:8888` にアクセス
- ユーザー名：`opsmini`
- パスワード：インストールスクリプトが表示するパスワード、または `/data/opsmini/.init_passwd` を確認

> ⚠️ パスワードはすぐに記録し、ログイン後に変更してください。再インストール時はパスワードが自動リセットされ再表示されます。

## 次のステップ

- [初回ログインと初期化](first-steps.md)
- [手動デプロイ（systemd）](../installation/manual-install.md)
- [リバースプロキシと HTTPS](../configuration/ports-proxy.md)
