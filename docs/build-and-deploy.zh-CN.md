<div align="center">

# OpsMini 构建与部署

**构建与部署手册**

[English](build-and-deploy.md) · [简体中文](build-and-deploy.zh-CN.md) · [繁體中文](build-and-deploy.zh-TW.md) · [日本語](build-and-deploy.ja.md) · [한국어](build-and-deploy.ko.md) · [ไทย](build-and-deploy.th.md) · [Deutsch](build-and-deploy.de.md)

</div>

---

> 版本：v1.0  
> 适用：从源码构建到生产部署的完整流程

---

## 1. 前置依赖

| 依赖 | 版本要求 | 说明 |
|------|----------|------|
| Go | **1.25+** | 纯 Go 驱动无 CGO，任意平台可直接交叉编译 |
| 内存 | ≥ 512MB | 构建与运行均轻量 |
| 目标主机 | Linux / macOS | 服务器场景面向 Linux x64 / arm64；macOS 仅用于开发调试 |

> **为什么无需 CGO 工具链**：SQLite 驱动采用 `github.com/glebarez/sqlite`（纯 Go 实现），
> 因此 `CGO_ENABLED=0` 即可在任意平台上交叉编译出**静态链接**的二进制，无需为每个目标架构单独配置 C 交叉编译器。

---

## 2. 获取代码与依赖

```bash
git clone <仓库地址> opsmini
cd opsmini

# 国内网络需配置 goproxy 镜像（proxy.golang.org 可能被墙）
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

---

## 3. 构建

### 3.1 编译当前平台

```bash
make build
# 产物：dist/opsmini
```

### 3.2 交叉编译全部目标平台

```bash
make build-all
# 产物：
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64
```

| 平台 | 适用场景 |
|------|----------|
| `linux/amd64` | 主流 x86_64 服务器（生产） |
| `linux/arm64` | ARM 服务器（Graviton / 树莓派 / 鲲鹏 / 飞腾，生产） |
| `darwin/amd64` | Intel Mac 开发机（开发调试） |
| `darwin/arm64` | Apple Silicon 开发机（开发调试） |

> OpsMini 是面向 **Linux 主机**的面板，仅发布 Linux 安装包；macOS 目标仅用于本地开发调试，不作为交付产物。

### 3.3 版本号注入

```bash
# 默认取 git tag / commit，也可显式指定
make build VERSION=v1.0.0

# 查看二进制版本
./dist/opsmini -version
# 输出：opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

> 正式发版流程：`git tag v1.0.0 && make build-all`，`VERSION` 自动取 tag 名。

### 3.4 其他 Makefile 目标

```bash
make clean     # 清理 dist/
make version   # 打印当前版本信息
```

### 3.5 校验产物架构

```bash
file dist/*
# 预期：ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64，均为静态链接
```

---

## 4. 配置

配置文件默认路径 `configs/config.yaml`（可用 `-config` 指定）。完整示例：

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

### 环境变量

| 变量 | 说明 |
|------|------|
| `OPSMINI_AI_KEY` | AI API Key，**优先于** `ai.api_key`（避免密钥落盘） |

---

## 5. 运行

```bash
# 直接运行
./opsmini -config configs/config.yaml

# 打印版本并退出
./opsmini -version
```

- 首次启动自动建表并写入默认管理员账号
- 访问 `http://<host>:8888` 即见面板（前端已 embed 进二进制，无需单独部署）

### 默认账号

| 项 | 值 |
|----|----|
| 用户名 | `opsmini`（固定） |
| 密码 | **首次启动时随机生成**，从启动日志查看 |

启动后日志会打印类似：

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

> ⚠️ 请立即记录该密码并登录后修改。密码仅在首次安装时生成一次，之后需通过面板内的「用户管理」修改。

---

## 6. 生产部署（Linux + systemd）

### 6.1 目录规划

```
/opt/opsmini/
├── opsmini              # 二进制
└── config.yaml          # 配置文件
/var/lib/opsmini/
└── opsmini.db           # 数据（自动生成，随 data 目录权限而定）
```

### 6.2 安装步骤

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

### 6.3 systemd 服务

创建 `/etc/systemd/system/opsmini.service`：

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

启用并启动：

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /var/lib/opsmini /opt/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

---

## 7. 反向代理（可选）

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

### 7.2 Caddy（自动 HTTPS）

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

### 7.3 安全入口

若配置了 `secret_entry: /opsmini_panel`，则面板路径变为 `http://<host>:8888/opsmini_panel`，可配合反代隐藏真实入口。

---

## 8. 升级

```bash
# 1. 备份数据
sudo systemctl stop opsmini
cp /var/lib/opsmini/opsmini.db /var/lib/opsmini/opsmini.db.bak.$(date +%s)

# 2. 替换二进制
sudo cp dist/opsmini-<new>-linux-amd64 /opt/opsmini/opsmini

# 3. 重启
sudo systemctl start opsmini
```

> SQLite 由 GORM 自动迁移，跨版本升级通常无需手动改表。

---

## 9. 常见问题

| 问题 | 原因 | 解决 |
|------|------|------|
| `go mod tidy` 卡住/报 Bad Gateway | `proxy.golang.org` 被墙 | `export GOPROXY=https://goproxy.cn,direct` |
| 提示 Go 版本过低 | 依赖要求 Go 1.25+ | 下载官方预编译二进制，见下 |
| 构建报 `vendor` 目录冲突 | 项目内 `vendor/` 与 Go 模块 vendor 约定冲突 | 前端依赖目录已改名 `static/`，勿再建 `vendor/` |
| 访问 `/` 返回 301 `./` | `c.FileFromFS` 对 embed.FS 的目录重定向 | 已改用 ReadFile + c.Data 修复（勿回退） |

### 安装 Go 1.25+（官方二进制，最快）

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

## 10. 交付产物清单

| 产物 | 说明 |
|------|------|
| `opsmini` 单二进制 | 后端 API + 前端 UI + SQLite，约 28~30MB，静态链接 |
| `configs/config.yaml` | 配置模板 |
| `opsmini.db` | 首次运行自动生成的数据文件 |

> 部署即拷贝二进制 + 配置文件，无其他运行时依赖。
