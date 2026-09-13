# ワンクリックスクリプトインストール

公式 `install.sh` を使用して Linux ホストにワンクリックでデプロイします。

## クイックスタート

```bash
# ワンクリックインストール（推奨。スクリプトがアーキテクチャに応じて Alibaba Cloud OSS からバイナリを自動ダウンロード）
curl -fsSL https://opsmini.com/install.sh | sudo bash

# ローカルバイナリを指定してインストール（デフォルトで /data/opsmini にインストール、ポート 8888）
sudo ./install.sh -b ./opsmini

# ダウンロード先をカスタマイズ（生のバイナリまたは .tar.gz に対応）
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# ディレクトリとポートをカスタマイズ
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## パラメータ

| パラメータ | 説明 | デフォルト値 |
|------|------|--------|
| `-d, --dir <path>` | インストールディレクトリ | `/data/opsmini` |
| `-p, --port <port>` | パネルのリッスンポート | `8888` |
| `-b, --binary <path>` | opsmini バイナリのパス | 未指定時は OSS からダウンロード |
| `-u, --url <url>` | URL からダウンロード（生のバイナリまたは `.tar.gz`） | OSS 公式アドレス |
| `-n, --no-systemd` | systemd サービスを登録しない | — |
| `-h, --help` | ヘルプ | — |

## インストール時の動作

1. インストールディレクトリを作成（デフォルト `/data/opsmini`）
2. バイナリをダウンロード / コピーして `<ディレクトリ>/opsmini` に配置（デフォルトでアーキテクチャに応じて Alibaba Cloud OSS からダウンロード）
3. `<ディレクトリ>/config.yaml` を生成（ポート、SQLite パス、JWT 有効期間、ログレベルとログファイル。JWT シークレットと Agent Token は初回起動時に自動生成して DB に保存）
4. 16 桁のランダム管理者パスワードを生成し、`<ディレクトリ>/.init_passwd`（権限 600）に書き込み
   - **新規インストール**：初回起動時に `OPSMINI_INIT_PASSWORD` 環境変数で注入
   - **再インストール**（データベースが既に存在）：自動で `-reset-pass` によりパスワードをリセット。出力されたパスワードが有効なパスワードになります
5. systemd サービス `opsmini.service` を登録して起動
6. サービスの応答をプローブし、アクセスアドレス / ユーザー名 / パスワード / ログファイルのパスを表示

## 成果物

```
/data/opsmini/
├── opsmini              # バイナリ
├── config.yaml          # 設定（600）
├── opsmini.db           # SQLite データベース（初回起動後に生成）
├── opsmini.log          # 実行ログファイル
└── .init_passwd         # 初期パスワード（600）
```

## バイナリの入手

- **公式配布（Alibaba Cloud OSS）**：`https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v<バージョン>-linux-<アーキテクチャ>`。スクリプトはデフォルトでここからダウンロードします
- **ローカル開発**：プロジェクトルートで `make build-all` を実行すると `dist/opsmini-<ver>-linux-amd64` / `-arm64` が生成されます。スクリプトはアーキテクチャに応じて自動で探します

## 互換性

- `x86_64` / `aarch64` のみ対応
- systemd のない環境では `-n` でサービス登録をスキップし、手動起動に切り替えます

## アカウント復旧 {: #account-recovery }

パスワードを忘れたり MFA 検証コードを紛失した場合は、サーバー上で実行します（先にサービスを停止し、操作後に再起動）：

```bash
# パスワードをリセット
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-pass opsmini
systemctl start opsmini

# MFA バインディングを解除
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-mfa opsmini
systemctl start opsmini
```

> 説明：`-reset-mfa` / `-reset-pass` はアカウント復旧用のサブコマンドで、SQLite データベースを直接操作してから終了し、Web サービスは起動しません。操作時は必ずサービスが停止状態であることを確認してください。
