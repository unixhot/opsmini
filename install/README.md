# OpsMini 安装脚本

`install.sh` 用于在 Linux 主机上一键安装 OpsMini。

## 快速开始

```bash
# 一键安装（推荐，自动按架构从官网下载二进制）
curl -fsSL https://opsmini.com/install.sh | sudo bash

# 指定本地二进制安装（默认安装到 /data/opsmini，端口 8888）
sudo ./install.sh -b ./opsmini

# 从 URL 下载并安装（支持 .tar.gz）
sudo ./install.sh -u https://example.com/opsmini-v1.0.0-linux-amd64.tar.gz

# 自定义目录与端口
sudo ./install.sh -b ./opsmini -d /opt/opsmini -p 9999
```

## 参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-d, --dir <path>` | 安装目录 | `/data/opsmini` |
| `-p, --port <port>` | 面板监听端口 | `8888` |
| `-b, --binary <path>` | opsmini 二进制路径 | 自动查找 |
| `-u, --url <url>` | 从 URL 下载（`.tar.gz` 或裸二进制） | — |
| `-n, --no-systemd` | 不注册 systemd 服务 | — |
| `-h, --help` | 帮助 | — |

## 安装行为

1. 创建安装目录（默认 `/data/opsmini`）
2. 复制 / 下载二进制到 `<目录>/opsmini`
3. 生成 `<目录>/config.yaml`（端口、SQLite 路径；JWT secret 与 Agent token 首次启动时自动生成并存入数据库）
4. 生成 16 位随机管理员密码，写入 `<目录>/.init_passwd`（权限 600）
5. 注册并启动 systemd 服务 `opsmini.service`（`OPSMINI_INIT_PASSWORD` 环境变量在首次启动时注入初始密码，已有账号时自动忽略）
6. 探测服务响应，并打印访问地址 / 用户名 / 密码

## 产物

```
/data/opsmini/
├── opsmini              # 二进制
├── config.yaml          # 配置（600）
├── opsmini.db           # SQLite 数据库（首次启动后生成）
└── .init_passwd         # 初始密码（600，首次启动后不再变化）
```

## 二进制获取

- 本地开发：项目根目录 `make build-all` 产出 `dist/opsmini-<ver>-linux-amd64` / `-arm64`，脚本会按架构自动查找脚本同目录、`dist/`、上层 `dist/` 下的产物
- 发布后：用 `-u` 指向 GitHub Release 的 `.tar.gz` 地址

## 兼容性

- 仅支持 `x86_64` / `aarch64`
- 无 systemd 的环境用 `-n` 跳过服务注册，改用手动启动

## 账号恢复（忘记密码 / 丢失 MFA 验证码）

### 忘记密码

在服务器上执行以下命令，把 `opsmini` 换成实际用户名（先停服务，操作完再启动）：

```bash
# 停服务
systemctl stop opsmini

# 重置密码（会生成并打印一个随机新密码）
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-pass opsmini

# 启动服务
systemctl start opsmini
```

执行后会输出一行 `已重置用户 xxx 的密码为：xxxx`，用该新密码登录面板后，请在「系统管理 - 用户」中自行修改密码。

### 丢失 MFA 双因素验证码

手机丢失、换机、卸载验证器导致无法获取动态码时，在服务器上执行：

```bash
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-mfa opsmini
systemctl start opsmini
```

该命令会清除指定用户的 MFA 绑定（关闭双因素验证），之后该用户可仅凭密码重新登录，并在「面板设置 - 双因素验证」中重新扫码绑定。

> 说明：`-reset-mfa` / `-reset-pass` 是账号恢复子命令，直接操作 SQLite 数据库后即退出，不会启动 Web 服务，务必保证操作时服务处于停止状态，避免数据库写入冲突。

### 初始密码

安装时生成的初始密码保存在 `/data/opsmini/.init_passwd`（权限 600），首次登录后请在面板中修改。
