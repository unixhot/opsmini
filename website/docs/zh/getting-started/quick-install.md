# 5 分钟快速安装

使用官方安装脚本 `opsmini-install.sh`，在 Linux 主机上一键完成部署。

## 前提

- 目标主机：Linux `x86_64` 或 `aarch64`
- 拥有 `sudo` 权限
- 可访问外网（用于下载二进制）或提前准备二进制文件

## 一键安装

```bash
# 方式一：从 URL 下载并安装（发布后使用 GitHub Release 地址）
sudo ./opsmini-install.sh -u https://example.com/opsmini-v1.0.0-linux-amd64.tar.gz

# 方式二：指定本地二进制安装（默认安装到 /data/opsmini，端口 8888）
sudo ./opsmini-install.sh -b ./opsmini

# 自定义目录与端口
sudo ./opsmini-install.sh -b ./opsmini -d /opt/opsmini -p 9999
```

## 安装参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-d, --dir <path>` | 安装目录 | `/data/opsmini` |
| `-p, --port <port>` | 面板监听端口 | `8888` |
| `-b, --binary <path>` | opsmini 二进制路径 | 自动查找 |
| `-u, --url <url>` | 从 URL 下载（`.tar.gz` 或裸二进制） | — |
| `-n, --no-systemd` | 不注册 systemd 服务 | — |

## 安装行为

1. 创建安装目录（默认 `/data/opsmini`）
2. 复制 / 下载二进制到 `<目录>/opsmini`
3. 生成 `<目录>/config.yaml`（随机 JWT secret、随机 Agent token、端口、SQLite 路径）
4. 生成 16 位随机管理员密码，写入 `<目录>/.init_passwd`（权限 600）
5. 注册并启动 systemd 服务 `opsmini.service`
6. 探测服务响应，打印访问地址 / 用户名 / 密码

## 产物结构

```
/data/opsmini/
├── opsmini              # 二进制
├── config.yaml          # 配置（600）
├── opsmini.db           # SQLite 数据库（首次启动后生成）
└── .init_passwd         # 初始密码（600）
```

## 首次登录

启动后日志会打印类似：

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

- 访问 `http://<host>:8888`
- 用户名：`opsmini`
- 密码：见启动日志或 `/data/opsmini/.init_passwd`

> ⚠️ 请立即记录密码并登录后修改。密码仅首次安装时生成一次。

## 下一步

- [首次登录与初始化](first-steps.md)
- [手动部署（systemd）](../installation/manual-install.md)
- [反向代理与 HTTPS](../configuration/ports-proxy.md)
