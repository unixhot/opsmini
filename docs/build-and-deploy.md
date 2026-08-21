<div align="center">

# OpsMini Build & Deployment

**Build & Deployment Guide**

[English](build-and-deploy.md) · [简体中文](build-and-deploy.zh-CN.md) · [繁體中文](build-and-deploy.zh-TW.md) · [日本語](build-and-deploy.ja.md) · [한국어](build-and-deploy.ko.md) · [ไทย](build-and-deploy.th.md) · [Deutsch](build-and-deploy.de.md)

</div>

---

> Version: v1.0  
> Scope: the complete workflow from building from source to production deployment

---

## 1. Prerequisites

| Dependency | Version Requirement | Description |
|------|----------|------|
| Go | **1.25+** | Pure Go with no CGO; cross-compilation works directly on any platform |
| Memory | ≥ 512MB | Lightweight for both build and runtime |
| Target host | Linux / macOS | Server scenarios target Linux x64 / arm64; macOS is for development and debugging only |

> **Why no CGO toolchain is required**: the SQLite driver uses `github.com/glebarez/sqlite` (a pure Go implementation),
> so `CGO_ENABLED=0` can cross-compile a **statically linked** binary on any platform, without configuring a separate C cross-compiler for each target architecture.

---

## 2. Get the code and dependencies

```bash
git clone <仓库地址> opsmini
cd opsmini

# 国内网络需配置 goproxy 镜像（proxy.golang.org 可能被墙）
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

---

## 3. Build

### 3.1 Compile for the current platform

```bash
make build
# 产物：dist/opsmini
```

### 3.2 Cross-compile for all target platforms

```bash
make build-all
# 产物：
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64
```

| Platform | Use case |
|------|----------|
| `linux/amd64` | Mainstream x86_64 servers (production) |
| `linux/arm64` | ARM servers (Graviton / Raspberry Pi / Kunpeng / Phytium, production) |
| `darwin/amd64` | Intel Mac development machines (development & debugging) |
| `darwin/arm64` | Apple Silicon development machines (development & debugging) |

> OpsMini is a panel targeting **Linux hosts**; only Linux installers are released. macOS targets are for local development and debugging only, not for delivery.

### 3.3 Version injection

```bash
# 默认取 git tag / commit，也可显式指定
make build VERSION=v1.0.0

# 查看二进制版本
./dist/opsmini -version
# 输出：opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

> Official release flow: `git tag v1.0.0 && make build-all`; `VERSION` is automatically taken from the tag name.

### 3.4 Other Makefile targets

```bash
make clean     # 清理 dist/
make version   # 打印当前版本信息
```

### 3.5 Verify artifact architecture

```bash
file dist/*
# 预期：ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64，均为静态链接
```

---

## 4. Configuration

The default configuration file path is `configs/config.yaml` (can be specified with `-config`). Complete example:

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

### Environment variables

| Variable | Description |
|------|------|
| `OPSMINI_AI_KEY` | AI API Key, **takes precedence over** `ai.api_key` (avoids writing the key to disk) |

---

## 5. Run

```bash
# 直接运行
./opsmini -config configs/config.yaml

# 打印版本并退出
./opsmini -version
```

- On first startup, tables are created automatically and a default admin account is written
- Visit `http://<host>:8888` to see the panel (the frontend is embedded in the binary; no separate deployment needed)

### Default account

| Item | Value |
|----|----|
| Username | `opsmini` (fixed) |
| Password | **Randomly generated on first startup**; check the startup logs |

After startup, the log prints something like:

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

> ⚠️ Record this password immediately and change it after login. The password is generated only once at first install; afterwards it must be changed via "User Management" in the panel.

---

## 6. Production deployment (Linux + systemd)

### 6.1 Directory layout

```
/opt/opsmini/
├── opsmini              # 二进制
└── config.yaml          # 配置文件
/var/lib/opsmini/
└── opsmini.db           # 数据（自动生成，随 data 目录权限而定）
```

### 6.2 Installation steps

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

### 6.3 systemd service

Create `/etc/systemd/system/opsmini.service`:

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

Enable and start:

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /var/lib/opsmini /opt/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

---

## 7. Reverse proxy (optional)

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

### 7.2 Caddy (automatic HTTPS)

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

### 7.3 Secret entry

If `secret_entry: /opsmini_panel` is configured, the panel path becomes `http://<host>:8888/opsmini_panel`, which can hide the real entry when combined with a reverse proxy.

---

## 8. Upgrade

```bash
# 1. 备份数据
sudo systemctl stop opsmini
cp /var/lib/opsmini/opsmini.db /var/lib/opsmini/opsmini.db.bak.$(date +%s)

# 2. 替换二进制
sudo cp dist/opsmini-<new>-linux-amd64 /opt/opsmini/opsmini

# 3. 重启
sudo systemctl start opsmini
```

> SQLite is migrated automatically by GORM; cross-version upgrades usually require no manual table changes.

---

## 9. FAQ

| Problem | Cause | Solution |
|------|------|------|
| `go mod tidy` hangs / Bad Gateway | `proxy.golang.org` is blocked | `export GOPROXY=https://goproxy.cn,direct` |
| "Go version too low" error | Dependencies require Go 1.25+ | Download the official precompiled binary, see below |
| Build reports a `vendor` directory conflict | The project's `vendor/` conflicts with Go module vendor conventions | The frontend dependency directory has been renamed `static/`; do not create `vendor/` again |
| Accessing `/` returns 301 `./` | `c.FileFromFS` directory redirect for embed.FS | Fixed by using ReadFile + c.Data (do not revert) |

### Install Go 1.25+ (official binary, fastest)

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

## 10. Deliverables

| Artifact | Description |
|------|------|
| `opsmini` single binary | Backend API + frontend UI + SQLite, about 28~30MB, statically linked |
| `configs/config.yaml` | Configuration template |
| `opsmini.db` | Data file auto-generated on first run |

> Deployment is just copying the binary + config file; no other runtime dependencies.
