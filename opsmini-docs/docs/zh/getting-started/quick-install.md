# 5 分钟快速安装

使用官方安装脚本一键完成部署：Linux 用 `install.sh`，Windows 用 `install.ps1`（管理员 PowerShell）。

## 前提

- 目标主机：Linux `x86_64` / `aarch64`，或 Windows `amd64` / `arm64`
- Linux 需要 `sudo`；Windows 需要以**管理员身份**打开 PowerShell
- 可访问外网（用于下载二进制）或提前准备二进制文件

## 一键安装

```bash
# 一键安装（推荐，脚本自动按架构从阿里云 OSS 下载二进制）
curl -fsSL https://opsmini.com/install.sh | sudo bash
```

Windows（管理员 PowerShell）：

```powershell
iwr -useb https://opsmini.com/install.ps1 -OutFile $env:TEMP\opsmini.ps1; & $env:TEMP\opsmini.ps1
```

> Windows 上程序装在 `C:\Program Files\OpsMini`，数据（数据库、配置、日志、初始密码 `.init_passwd`）在 `C:\ProgramData\OpsMini`，注册的服务名为 `OpsMini`（SCM），同样开机自启。完整参数见[一键脚本安装](../installation/one-click-install.md)。

> **Windows 上暂不采集日志与登录事件**：这两类数据在事件日志（.evtx）里，需要 EvtQuery API，当前版本仍按文本 tail 实现。因此「日志暴涨」「SSH 爆破」「登录异常」三类检测不会触发，界面上也不会出现对应选项；进程、网络、指标、文件完整性照常工作。

Linux 的其它常用参数：

```bash
# 指定本地二进制安装（默认安装到 /data/opsmini，端口 8888）
sudo ./install.sh -b ./opsmini

# 自定义下载地址
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# 自定义目录与端口
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## 安装参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-d, --dir <path>` | 安装目录 | `/data/opsmini` |
| `-p, --port <port>` | 面板监听端口 | `8888` |
| `-b, --binary <path>` | opsmini 二进制路径 | 未指定时从 OSS 下载 |
| `-u, --url <url>` | 从 URL 下载（`.tar.gz` 或裸二进制） | OSS 官方地址 |
| `-n, --no-systemd` | 不注册 systemd 服务 | — |

## 安装行为

1. 创建安装目录（默认 `/data/opsmini`）
2. 下载 / 复制二进制到 `<目录>/opsmini`（默认按架构从阿里云 OSS 下载）
3. 生成 `<目录>/config.yaml`（端口、SQLite 路径、JWT 有效期、日志级别与日志文件；JWT 密钥与 Agent Token 首次启动自动生成存库）
4. 生成 16 位随机管理员密码，写入 `<目录>/.init_passwd`（权限 600）
5. 注册并启动系统服务：Linux 为 `opsmini.service`（systemd），Windows 为 `OpsMini`（SCM）
6. 探测服务响应，打印访问地址 / 用户名 / 密码 / 日志文件路径

## 产物结构

Linux（`/data/opsmini`）：

```
/data/opsmini/
├── opsmini              # 二进制
├── config.yaml          # 配置（600）
├── opsmini.db           # SQLite 数据库（首次启动后生成）
├── opsmini.log          # 运行日志文件
└── .init_passwd         # 初始密码（600）
```

Windows：程序在 `C:\Program Files\OpsMini\opsmini.exe`，数据目录 `C:\ProgramData\OpsMini\` 下是同样的 `config.yaml` / `opsmini.db` / `opsmini.log` / `.init_passwd`。

## 首次登录

- 访问 `http://<host>:8888`
- 用户名：`opsmini`
- 密码：安装脚本打印的密码，或查看 `.init_passwd`（Linux `/data/opsmini/`，Windows `C:\ProgramData\OpsMini\`）

> ⚠️ 请立即记录密码并登录后修改。重装时密码会自动重置并重新打印。

## 下一步

- [首次登录与初始化](first-steps.md)
- [手动部署（systemd）](../installation/manual-install.md)
- [反向代理与 HTTPS](../configuration/ports-proxy.md)
