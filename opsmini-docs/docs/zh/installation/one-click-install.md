# 一键脚本安装

官方脚本一键完成部署：Linux 使用 `install.sh`，Windows 使用 `install.ps1`（管理员 PowerShell）。

## 快速开始

```bash
# 一键安装（推荐，脚本自动按架构从阿里云 OSS 下载二进制）
curl -fsSL https://opsmini.com/install.sh | sudo bash
```

Windows（管理员 PowerShell，脚本自动按 amd64 / arm64 取包）：

```powershell
iwr -useb https://opsmini.com/install.ps1 -OutFile $env:TEMP\opsmini.ps1; & $env:TEMP\opsmini.ps1
```

```bash
# 以下为 Linux 的常用参数

# 指定本地二进制安装（默认安装到 /data/opsmini，端口 8888）
sudo ./install.sh -b ./opsmini

# 自定义下载地址（支持裸二进制或 .tar.gz）
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# 自定义目录与端口
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## 参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-d, --dir <path>` | 安装目录 | `/data/opsmini` |
| `-p, --port <port>` | 面板监听端口 | `8888` |
| `-b, --binary <path>` | opsmini 二进制路径 | 未指定时从 OSS 下载 |
| `-u, --url <url>` | 从 URL 下载（裸二进制或 `.tar.gz`） | OSS 官方地址 |
| `-n, --no-systemd` | 不注册 systemd 服务 | — |
| `-h, --help` | 帮助 | — |

### Windows 参数（install.ps1）

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-InstallDir <path>` | 程序目录 | `C:\Program Files\OpsMini` |
| `-DataDir <path>` | 数据目录（二进制、配置、数据库、日志都在这里） | `C:\ProgramData\OpsMini` |
| `-Port <port>` | 面板监听端口 | `8888` |
| `-Binary <path>` | 本地二进制（离线安装用） | 未指定时从 OSS 下载 |
| `-BaseUrl <url>` | 安装包目录 | 官方 OSS |
| `-Version <ver>` | 指定版本 | 当前发布版 |
| `-ServerAddr <host:port>` | 接入 OpsAnt 的 Server 地址 | — |
| `-ServerToken <token>` | 接入 OpsAnt 的 token | — |
| `-Uninstall` | 停止并移除服务（保留数据目录） | — |

```powershell
# 自定义端口与数据目录
.\install.ps1 -Port 9999 -DataDir D:\opsmini-data

# 离线安装
.\install.ps1 -Binary .\opsmini.exe

# 装完即接入 OpsAnt
.\install.ps1 -ServerAddr 47.104.172.52:4507 -ServerToken <token>
```

> Windows 上不注册 systemd，而是注册名为 `OpsMini` 的 Windows 服务（SCM），同样开机自启。脚本检测到非管理员运行时会自己提权——「应用和功能」里的卸载入口默认不带提权，只报错的话用户点完卸载看起来毫无反应。

## 安装行为

1. 创建安装目录（默认 `/data/opsmini`）
2. 下载 / 复制二进制到 `<目录>/opsmini`（默认按架构从阿里云 OSS 下载）
3. 生成 `<目录>/config.yaml`（端口、SQLite 路径、JWT 有效期、日志级别与日志文件；JWT 密钥与 Agent Token 首次启动自动生成存库）
4. 生成 16 位随机管理员密码，写入 `<目录>/.init_passwd`（权限 600）
   - **全新安装**：首次启动时通过 `OPSMINI_INIT_PASSWORD` 环境变量注入
   - **重装**（数据库已存在）：自动用 `-reset-pass` 重置密码，输出的密码即为生效密码
5. 注册并启动 systemd 服务 `opsmini.service`
6. 探测服务响应，并打印访问地址 / 用户名 / 密码 / 日志文件路径

## 产物

```
/data/opsmini/
├── opsmini              # 二进制
├── config.yaml          # 配置（600）
├── opsmini.db           # SQLite 数据库（首次启动后生成）
├── opsmini.log          # 运行日志文件
└── .init_passwd         # 初始密码（600）
```

## 二进制获取

- **官方分发（阿里云 OSS）**：`https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v<版本>-linux-<架构>`，脚本默认从此处下载
- **本地开发**：项目根目录 `make build-all` 产出 `dist/opsmini-<ver>-linux-amd64` / `-arm64`，脚本会按架构自动查找

## 兼容性

- Linux 支持 `x86_64` / `aarch64`，Windows 支持 `amd64` / `arm64`
- 无 systemd 的环境用 `-n` 跳过服务注册，改用手动启动

## 账号恢复 {: #account-recovery }

忘记密码或丢失 MFA 验证码时，在服务器上执行（先停服务，操作完再启动）：

```bash
# 重置密码
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-pass opsmini
systemctl start opsmini

# 清除 MFA 绑定
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-mfa opsmini
systemctl start opsmini
```

> 说明：`-reset-mfa` / `-reset-pass` 是账号恢复子命令，直接操作 SQLite 数据库后即退出，不会启动 Web 服务，务必保证操作时服务处于停止状态。
