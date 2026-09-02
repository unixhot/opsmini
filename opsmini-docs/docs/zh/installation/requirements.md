# 环境要求

## 目标主机

| 项 | 最低要求 | 推荐 |
|----|----------|------|
| 操作系统 | 主流 Linux 发行版（x86_64 / aarch64） | Ubuntu 20.04+ / Debian 11+ / CentOS 7+ |
| CPU | 1 核 | 2 核 |
| 内存 | 512 MB | 1 GB+ |
| 磁盘 | 200 MB（二进制 + 数据） | 1 GB+（含日志与 SQLite） |
| 网络 | 可访问外网（首次下载） | 稳定内网即可 |

## 架构支持

| 平台 | 用途 |
|------|------|
| `linux/amd64` | 主流 x86_64 服务器（生产） |
| `linux/arm64` | ARM 服务器（Graviton / 树莓派 / 鲲鹏 / 飞腾，生产） |
| `darwin/amd64` / `darwin/arm64` | macOS 开发机（仅开发调试，不交付） |

## 运行依赖

- **零运行时依赖**：前端、SQLite、静态资源全部嵌入二进制，无需安装 PHP / Node / 数据库
- Docker 管理功能需要宿主机已安装 Docker（面板本身不依赖 Docker）

## 权限

- 一键安装脚本需要 `root` 或 `sudo`
- 面板进程建议以专用低权限用户（如 `opsmini`）运行，详见 [systemd 托管](systemd.md)
