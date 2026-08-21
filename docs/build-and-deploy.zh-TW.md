<div align="center">

# OpsMini 建置與部署

**建置與部署手冊**

[English](build-and-deploy.md) · [简体中文](build-and-deploy.zh-CN.md) · [繁體中文](build-and-deploy.zh-TW.md) · [日本語](build-and-deploy.ja.md) · [한국어](build-and-deploy.ko.md) · [ไทย](build-and-deploy.th.md) · [Deutsch](build-and-deploy.de.md)

</div>

---

> 版本：v1.0  
> 適用：從原始碼建置到正式部署的完整流程

---

## 1. 前置依賴

| 依賴 | 版本要求 | 說明 |
|------|----------|------|
| Go | **1.25+** | 純 Go 驅動、無 CGO，任意平台皆可直接交叉編譯 |
| 記憶體 | ≥ 512MB | 建置與執行皆輕量 |
| 目標主機 | Linux / macOS | 伺服器情境面向 Linux x64 / arm64；macOS 僅用於開發偵錯 |

> **為何不需要 CGO 工具鏈**：SQLite 驅動採用 `github.com/glebarez/sqlite`（純 Go 實作），
> 因此 `CGO_ENABLED=0` 即可在任意平台上交叉編譯出**靜態連結**的二進位檔，無需為每個目標架構單獨設定 C 交叉編譯器。

---

## 2. 取得程式碼與依賴

```bash
git clone <仓库地址> opsmini
cd opsmini

# 国内网络需配置 goproxy 镜像（proxy.golang.org 可能被墙）
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

---

## 3. 建置

### 3.1 編譯目前平台

```bash
make build
# 产物：dist/opsmini
```

### 3.2 交叉編譯所有目標平台

```bash
make build-all
# 产物：
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64
```

| 平台 | 適用情境 |
|------|----------|
| `linux/amd64` | 主流 x86_64 伺服器（正式） |
| `linux/arm64` | ARM 伺服器（Graviton / 樹莓派 / 鯤鵬 / 飛騰，正式） |
| `darwin/amd64` | Intel Mac 開發機（開發偵錯） |
| `darwin/arm64` | Apple Silicon 開發機（開發偵錯） |

> OpsMini 是面向 **Linux 主機**的面板，僅發布 Linux 安裝套件；macOS 目標僅用於本機開發偵錯，不作為交付產物。

### 3.3 版本號注入

```bash
# 默认取 git tag / commit，也可显式指定
make build VERSION=v1.0.0

# 查看二进制版本
./dist/opsmini -version
# 输出：opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

> 正式發版流程：`git tag v1.0.0 && make build-all`，`VERSION` 自動取自 tag 名稱。

### 3.4 其他 Makefile 目標

```bash
make clean     # 清理 dist/
make version   # 打印当前版本信息
```

### 3.5 驗證產物架構

```bash
file dist/*
# 预期：ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64，均为静态链接
```

---

## 4. 設定

設定檔預設路徑為 `configs/config.yaml`（可用 `-config` 指定）。完整範例：

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

### 環境變數

| 變數 | 說明 |
|------|------|
| `OPSMINI_AI_KEY` | AI API Key，**優先於** `ai.api_key`（避免金鑰寫入磁碟） |

---

## 5. 執行

```bash
# 直接运行
./opsmini -config configs/config.yaml

# 打印版本并退出
./opsmini -version
```

- 首次啟動自動建立資料表並寫入預設管理員帳號
- 存取 `http://<host>:8888` 即可看到面板（前端已 embed 進二進位檔，無需另外部署）

### 預設帳號

| 項目 | 值 |
|----|----|
| 使用者名稱 | `opsmini`（固定） |
| 密碼 | **首次啟動時隨機產生**，請從啟動日誌查看 |

啟動後日誌會列印類似：

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

> ⚠️ 請立即記錄該密碼，並於登入後修改。密碼僅在首次安裝時產生一次，之後需透過面板內的「使用者管理」修改。

---

## 6. 正式部署（Linux + systemd）

### 6.1 目錄規劃

```
/opt/opsmini/
├── opsmini              # 二进制
└── config.yaml          # 配置文件
/var/lib/opsmini/
└── opsmini.db           # 数据（自动生成，随 data 目录权限而定）
```

### 6.2 安裝步驟

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

### 6.3 systemd 服務

建立 `/etc/systemd/system/opsmini.service`：

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

啟用並啟動：

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /var/lib/opsmini /opt/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

---

## 7. 反向代理（選用）

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

### 7.3 安全入口

若設定了 `secret_entry: /opsmini_panel`，面板路徑將變為 `http://<host>:8888/opsmini_panel`，可配合反向代理隱藏真實入口。

---

## 8. 升級

```bash
# 1. 备份数据
sudo systemctl stop opsmini
cp /var/lib/opsmini/opsmini.db /var/lib/opsmini/opsmini.db.bak.$(date +%s)

# 2. 替换二进制
sudo cp dist/opsmini-<new>-linux-amd64 /opt/opsmini/opsmini

# 3. 重启
sudo systemctl start opsmini
```

> SQLite 由 GORM 自動遷移，跨版本升級通常無需手動改表。

---

## 9. 常見問題

| 問題 | 原因 | 解決 |
|------|------|------|
| `go mod tidy` 卡住/回報 Bad Gateway | `proxy.golang.org` 被牆 | `export GOPROXY=https://goproxy.cn,direct` |
| 提示 Go 版本過低 | 依賴要求 Go 1.25+ | 下載官方預編譯二進位檔，見下方 |
| 建置回報 `vendor` 目錄衝突 | 專案內 `vendor/` 與 Go 模組 vendor 約定衝突 | 前端依賴目錄已改名為 `static/`，勿再建立 `vendor/` |
| 存取 `/` 回傳 301 `./` | `c.FileFromFS` 對 embed.FS 的目錄重導向 | 已改用 ReadFile + c.Data 修復（勿回退） |

### 安裝 Go 1.25+（官方二進位檔，最快）

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

## 10. 交付產物清單

| 產物 | 說明 |
|------|------|
| `opsmini` 單一二進位檔 | 後端 API + 前端 UI + SQLite，約 28~30MB，靜態連結 |
| `configs/config.yaml` | 設定範本 |
| `opsmini.db` | 首次執行自動產生的資料檔案 |

> 部署即複製二進位檔 + 設定檔，無其他執行階段依賴。
