<div align="center">

# OpsMini ビルドとデプロイ

**ビルドとデプロイガイド**

[English](build-and-deploy.md) · [简体中文](build-and-deploy.zh-CN.md) · [繁體中文](build-and-deploy.zh-TW.md) · [日本語](build-and-deploy.ja.md) · [한국어](build-and-deploy.ko.md) · [ไทย](build-and-deploy.th.md) · [Deutsch](build-and-deploy.de.md)

</div>

---

> バージョン：v1.0  
> 対象：ソースコードからのビルドから本番デプロイまでの完全な流れ

---

## 1. 前提条件

| 依存 | バージョン要件 | 説明 |
|------|----------|------|
| Go | **1.25+** | 純 Go 実装で CGO 不要。任意のプラットフォームで直接クロスコンパイル可能 |
| メモリ | ≥ 512MB | ビルド・実行ともに軽量 |
| 対象ホスト | Linux / macOS | サーバー用途は Linux x64 / arm64 向け。macOS は開発・デバッグ専用 |

> **CGO ツールチェーンが不要な理由**：SQLite ドライバは `github.com/glebarez/sqlite`（純 Go 実装）を採用しているため、
> `CGO_ENABLED=0` で任意のプラットフォーム上に**静的リンク**のバイナリをクロスコンパイルできます。ターゲットアーキテクチャごとに C クロスコンパイラを個別設定する必要はありません。

---

## 2. コードと依存関係の取得

```bash
git clone <仓库地址> opsmini
cd opsmini

# 国内网络需配置 goproxy 镜像（proxy.golang.org 可能被墙）
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

---

## 3. ビルド

### 3.1 現在のプラットフォーム向けコンパイル

```bash
make build
# 产物：dist/opsmini
```

### 3.2 全ターゲットプラットフォームのクロスコンパイル

```bash
make build-all
# 产物：
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64
```

| プラットフォーム | 用途 |
|------|----------|
| `linux/amd64` | 主流の x86_64 サーバー（本番） |
| `linux/arm64` | ARM サーバー（Graviton / Raspberry Pi / Kunpeng / Phytium、本番） |
| `darwin/amd64` | Intel Mac 開発マシン（開発・デバッグ） |
| `darwin/arm64` | Apple Silicon 開発マシン（開発・デバッグ） |

> OpsMini は **Linux ホスト**向けパネルで、Linux インストールパッケージのみ配布します。macOS ターゲットはローカル開発・デバッグ専用で、成果物としては提供しません。

### 3.3 バージョン番号の注入

```bash
# 默认取 git tag / commit，也可显式指定
make build VERSION=v1.0.0

# 查看二进制版本
./dist/opsmini -version
# 输出：opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

> 正式リリースの流れ：`git tag v1.0.0 && make build-all` を実行すると、`VERSION` は自動的にタグ名になります。

### 3.4 その他の Makefile ターゲット

```bash
make clean     # 清理 dist/
make version   # 打印当前版本信息
```

### 3.5 成果物アーキテクチャの確認

```bash
file dist/*
# 预期：ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64，均为静态链接
```

---

## 4. 設定

設定ファイルのデフォルトパスは `configs/config.yaml`（`-config` で指定可能）。完全な例：

```yaml
server:
  host: "0.0.0.0"              # 监听地址
  port: 8888                    # 监听端口
  secret_entry: ""              # 安全入口前缀，如 /opsmini_panel（留空不启用）

database:
  path: "opsmini.db"            # SQLite 数据文件路径

jwt:
  secret: "change-me"           # JWT 签名密钥，生产环境务必修改为随机串
  access_ttl_seconds: 900       # access token 有效期（15 分钟）
  refresh_ttl_seconds: 604800   # refresh token 有效期（7 天）

ai:
  enabled: true                 # 是否启用 AI 助手
  provider: "openai"            # openai / deepseek / qwen / ollama
  model: "gpt-4o"               # 模型名
  base_url: "https://api.openai.com/v1"
  api_key: ""                   # 建议用环境变量 OPSMINI_AI_KEY 覆盖

agent:
  token: ""                     # 对外 REST API 认证令牌，空则禁用 /agent/v1
  allowed_commands:             # 命令白名单前缀（仅允许以此开头的命令）
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

### 環境変数

| 変数 | 説明 |
|------|------|
| `OPSMINI_AI_KEY` | AI API Key。`ai.api_key` より**優先**されます（キーのディスク保存を回避） |

---

## 5. 実行

```bash
# 直接运行
./opsmini -config configs/config.yaml

# 打印版本并退出
./opsmini -version
```

- 初回起動時にテーブルを自動作成し、デフォルトの管理者アカウントを書き込みます
- `http://<host>:8888` にアクセスするとパネルが表示されます（フロントエンドはバイナリに組み込み済みで、個別デプロイは不要）

### デフォルトアカウント

| 項目 | 値 |
|----|----|
| ユーザー名 | `opsmini`（固定） |
| パスワード | **初回起動時にランダム生成**。起動ログで確認 |

起動後のログには次のように出力されます：

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

> ⚠️ このパスワードをすぐに記録し、ログイン後に変更してください。パスワードは初回インストール時に一度だけ生成され、以降はパネル内の「ユーザー管理」から変更する必要があります。

---

## 6. 本番デプロイ（Linux + systemd）

### 6.1 ディレクトリ構成

```
/opt/opsmini/
├── opsmini              # 二进制
└── config.yaml          # 配置文件
/var/lib/opsmini/
└── opsmini.db           # 数据（自动生成，随 data 目录权限而定）
```

### 6.2 インストール手順

```bash
# 1. 放置二进制与配置
sudo mkdir -p /opt/opsmini /var/lib/opsmini
sudo cp dist/opsmini-*-linux-amd64 /opt/opsmini/opsmini
sudo cp configs/config.yaml /opt/opsmini/config.yaml

# 2. 修改配置：JWT secret、数据库绝对路径
sudo sed -i 's|path: "opsmini.db"|path: "/var/lib/opsmini/opsmini.db"|' /opt/opsmini/config.yaml
sudo sed -i 's|secret: "change-me"|secret: "<随机长串>"|' /opt/opsmini/config.yaml

# 3. 赋权
sudo chmod +x /opt/opsmini/opsmini
```

### 6.3 systemd サービス

`/etc/systemd/system/opsmini.service` を作成します：

```ini
[Unit]
Description=OpsMini Server Panel
After=network.target

[Service]
Type=simple
ExecStart=/opt/opsmini/opsmini -config /opt/opsmini/config.yaml
Restart=always
RestartSec=3
# 安全加固（可选）
User=opsmini
Group=opsmini
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

有効化して起動：

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /var/lib/opsmini /opt/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

---

## 7. リバースプロキシ（任意）

### 7.1 Nginx

```nginx
server {
    listen 80;
    server_name panel.example.com;

    location / {
        proxy_pass http://127.0.0.1:8888;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Web 终端（WebSocket）需要升级支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### 7.2 Caddy（自動 HTTPS）

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

### 7.3 シークレットエントリ

`secret_entry: /opsmini_panel` を設定すると、パネルのパスは `http://<host>:8888/opsmini_panel` になり、リバースプロキシと組み合わせて実際の入口を隠せます。

---

## 8. アップグレード

```bash
# 1. 备份数据
sudo systemctl stop opsmini
cp /var/lib/opsmini/opsmini.db /var/lib/opsmini/opsmini.db.bak.$(date +%s)

# 2. 替换二进制
sudo cp dist/opsmini-<new>-linux-amd64 /opt/opsmini/opsmini

# 3. 重启
sudo systemctl start opsmini
```

> SQLite は GORM が自動マイグレーションするため、バージョンをまたぐアップグレードでも通常は手動でテーブルを変更する必要はありません。

---

## 9. よくある質問

| 問題 | 原因 | 解決策 |
|------|------|------|
| `go mod tidy` が停止/Bad Gateway | `proxy.golang.org` が遮断されている | `export GOPROXY=https://goproxy.cn,direct` |
| Go のバージョンが低すぎるエラー | 依存が Go 1.25+ を要求 | 公式のプリコンパイル済みバイナリをダウンロード（下記参照） |
| ビルドで `vendor` ディレクトリの競合 | プロジェクト内 `vendor/` と Go モジュールの vendor 規約の競合 | フロントエンドの依存ディレクトリは `static/` に改名済み。`vendor/` を再作成しない |
| `/` へのアクセスが 301 `./` を返す | `c.FileFromFS` の embed.FS ディレクトリリダイレクト | ReadFile + c.Data で修正済み（戻さない） |

### Go 1.25+ のインストール（公式バイナリ、最速）

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

---

## 10. 成果物リスト

| 成果物 | 説明 |
|------|------|
| `opsmini` 単一バイナリ | バックエンド API + フロントエンド UI + SQLite、約 28〜30MB、静的リンク |
| `configs/config.yaml` | 設定テンプレート |
| `opsmini.db` | 初回実行時に自動生成されるデータファイル |

> デプロイはバイナリ + 設定ファイルのコピーだけで、その他のランタイム依存はありません。
