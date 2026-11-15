# OpsMini 安装脚本

本目录存放 OpsMini 的两个平台安装脚本，它们同时发布在官网同一位置：

| 脚本 | 平台 | 官网地址 |
|---|---|---|
| `install.sh` | Linux（amd64 / arm64） | `https://opsmini.com/install.sh` |
| `install.ps1` | Windows（amd64 / arm64） | `https://opsmini.com/install.ps1` |

两者的安装结果一致：注册为系统服务、开机自启、生成配置文件与初始管理员密码。区别只在服务托管方式（systemd / Windows 服务）和目录约定。

> 这两个脚本只做一件事：**从阿里云 OSS 按版本号下载对应平台的安装包，然后安装**。不涉及 OpsAnt，也不做接入配置。
>
> OpsAnt / 管控平台环境里下发的「一键安装」用的是**另一套脚本**（安装包从 OpsAnt 自身下载，并自动写入 OpsAnt 接入信息），与本目录无关。两者不要混用，也不要互相覆盖，详见文末「与 OpsAnt 侧安装脚本的区别」。

## 快速开始

### Linux

```bash
# 一键安装（推荐，自动按架构从官网下载二进制）
curl -fsSL https://opsmini.com/install.sh | sudo bash

# 指定本地二进制安装（默认安装到 /data/opsmini，端口 8888）
sudo ./install.sh -b ./opsmini

# 从 URL 下载并安装（支持 .tar.gz）
sudo ./install.sh -u https://example.com/opsmini-v1.5.0-linux-amd64.tar.gz

# 自定义目录与端口
sudo ./install.sh -b ./opsmini -d /opt/opsmini -p 9999
```

### Windows（管理员 PowerShell）

```powershell
# 一键安装（自动按 PROCESSOR_ARCHITECTURE 选择 amd64 / arm64 包）
iwr -useb https://opsmini.com/install.ps1 -OutFile $env:TEMP\opsmini.ps1; & $env:TEMP\opsmini.ps1

# 指定本地二进制与安装目录
.\install.ps1 -Binary .\opsmini.exe -InstallDir D:\OpsMini

# 自定义端口
.\install.ps1 -Port 9999
```

脚本会先校验管理员权限，非管理员直接报错退出，不会留下半装状态。

## 参数

### Linux — `install.sh`

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-d, --dir <path>` | 安装目录 | `/data/opsmini` |
| `-p, --port <port>` | 面板监听端口 | `8888` |
| `-b, --binary <path>` | opsmini 二进制路径 | 自动查找 |
| `-u, --url <url>` | 从 URL 下载（`.tar.gz` 或裸二进制） | — |
| `--base-url <url>` | 内网 / 离线场景：从指定目录取包，优先于官方 OSS | — |
| `--version <vX.Y.Z>` | 指定版本号（决定下载文件名） | `v1.5.0` |
| `--server-addr <host:port>` | 接入 OpsAnt 的 OpsMini Server 地址（默认端口 `4507`） | — |
| `--server-token <token>` | 接入 OpsAnt 的 token，与 OpsAnt 的 `OPSMINI_TOKEN` 一致 | — |
| `-n, --no-systemd` | 不注册 systemd 服务（仅安装 + 写配置，不启动） | — |
| `-U, --uninstall` | 卸载：停止并移除 systemd 服务（保留数据目录） | — |
| `--purge` | 配合 `--uninstall`：连数据目录一并删除（不可恢复） | — |
| `-h, --help` | 显示帮助 | — |

### Windows — `install.ps1`

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-InstallDir <path>` | 程序目录（二进制与配置） | `C:\Program Files\OpsMini` |
| `-DataDir <path>` | 数据目录（SQLite、初始密码） | `C:\ProgramData\OpsMini` |
| `-Port <port>` | 面板监听端口 | `8888` |
| `-Binary <path>` | 本地 `opsmini.exe` 路径 | 脚本同目录的 `opsmini.exe` |
| `-BaseUrl <url>` | 内网 / 离线场景：安装包所在目录，优先于官方 OSS | — |
| `-BinaryUrl <url>` | 直接指定下载地址，优先级最高 | — |
| `-Version <vX.Y.Z>` | 指定版本号（决定下载文件名） | `v1.5.0` |
| `-ServerAddr <host:port>` | 接入 OpsAnt 的 OpsMini Server 地址 | — |
| `-ServerToken <token>` | 接入 OpsAnt 的 token | — |
| `-Uninstall` | 停止并移除服务（保留数据目录） | — |

安装包按文件名规则解析：`opsmini-<version>-windows-<arch>.exe`，`arch` 取自 `PROCESSOR_ARCHITECTURE`（`ARM64` → `arm64`，其余 → `amd64`）。

## 安装行为

### Linux

1. 创建安装目录（默认 `/data/opsmini`）
2. 复制 / 下载二进制到 `<目录>/opsmini`
3. 生成 `<目录>/config.yaml`（端口、SQLite 路径；JWT secret 与 Agent token 首次启动时自动生成并存入数据库）
4. 生成 16 位随机管理员密码，写入 `<目录>/.init_passwd`（权限 600）
5. 注册并启动 systemd 服务 `opsmini.service`（`OPSMINI_INIT_PASSWORD` 环境变量在首次启动时注入初始密码，已有账号时自动忽略）
6. 探测服务响应，并打印访问地址 / 用户名 / 密码

### Windows

1. 校验管理员权限
2. 定位或下载 `opsmini.exe`（`-Binary` → `-BinaryUrl` → `-BaseUrl`/官方 OSS，逐级回退）
3. 复制到 `<InstallDir>\opsmini.exe`
4. 生成 `<InstallDir>\config.yaml`；**已存在时**不覆盖，但只要传了 `-ServerAddr` 就就地改写 `agent` 段
5. 注册服务 `OpsMini`（`LocalSystem`、自动启动、显示名「OpsMini 运维面板」）；已存在则只更新启动参数（走升级路径）
6. 首次安装时放行入站 TCP 端口（防火墙规则 `OpsMini Panel`）
7. 启动服务并等待 `Running`
8. 打印访问地址、接入状态，以及从 `<DataDir>\.init_passwd` 读取的初始密码

## 产物

### Linux

```
/data/opsmini/
├── opsmini              # 二进制
├── config.yaml          # 配置（600）
├── opsmini.db           # SQLite 数据库（首次启动后生成）
└── .init_passwd         # 初始密码（600，首次启动后不再变化）
```

### Windows

```
C:\Program Files\OpsMini\
├── opsmini.exe          # 二进制（单文件，无需运行库）
└── config.yaml          # 配置
C:\ProgramData\OpsMini\
├── opsmini.db           # SQLite 数据库（首次启动后生成）
└── .init_passwd         # 初始密码（首次启动后生成）
```

程序与数据分离，升级时替换 exe 不影响数据；卸载时数据目录默认保留。

## 二进制获取

优先级（两个脚本一致）：本地文件 > `-u` / `-BinaryUrl` > `--base-url` / `-BaseUrl` > 官方 OSS。

本地开发用 `make build-all` 产出 `dist/` 下的四个平台产物，`install.sh` 会按当前架构在脚本同目录、`dist/`、上层 `dist/` 中自动查找：

```
dist/opsmini-<ver>-linux-amd64
dist/opsmini-<ver>-linux-arm64
dist/opsmini-<ver>-windows-amd64.exe
dist/opsmini-<ver>-windows-arm64.exe
```

命名规则固定：`opsmini-<version>-<os>-<arch>`，Windows 带 `.exe` 后缀。脚本按此规则拼下载地址，名字不符会 404。

Windows 目标在编译前会自动跑 `make winres`，用 `goversioninfo` 生成版本资源（`cmd/agent/resource_windows_*.syso`，不进仓库）。所以打出来的 exe 右键「属性 → 详细信息」能看到版本、公司、文件说明，和命令行 `-version` 输出一致。

## 可选：接入 OpsAnt

默认安装是**独立单机面板**，不与任何中心通信。如果确实要把它纳管到 OpsAnt，两个脚本都提供了接入参数，作用只是把地址与 token 写进 `config.yaml` 的 `agent` 段——**安装包依然从阿里云 OSS 下载，脚本本身不依赖 OpsAnt**。服务启动后自行注册，不需要再进面板手工填。

```bash
# Linux
curl -fsSL https://opsmini.com/install.sh | sudo bash -s -- \
  --server-addr 47.104.172.52:4507 --server-token <OPSMINI_TOKEN>
```

```powershell
# Windows
.\install.ps1 -ServerAddr 47.104.172.52:4507 -ServerToken <OPSMINI_TOKEN>
```

注意地址填 OpsAnt 的 **OpsMini Server gRPC 端口（默认 4507）**，不是 Web 端口 8010 —— 填错时面板会在「面板设置 - 接入 OpsAnt」显示「该端口返回的是 HTTP 响应，不是 gRPC」。注册成功后需在 OpsAnt 的「待纳管」里同意，主机才会进入列表。

由 OpsAnt 主动下发的部署（主机管理里的「一键安装」）用的是另一套脚本，见文末。

## 升级与卸载

**升级**：在旧版本上重复执行同一条安装命令即可。两个脚本都会识别已安装状态——只替换二进制、重启服务，不重置密码、不覆盖已有配置。

```bash
# Linux
curl -fsSL https://opsmini.com/install.sh | sudo bash

# Windows
iwr -useb https://opsmini.com/install.ps1 -OutFile $env:TEMP\opsmini.ps1; & $env:TEMP\opsmini.ps1
```

**卸载**：

```bash
# Linux：停服务并移除 systemd 单元，保留数据
sudo ./install.sh --uninstall

# 连数据目录一起删（不可恢复）
sudo ./install.sh --uninstall --purge
```

```powershell
# Windows：停止并删除服务，数据目录 C:\ProgramData\OpsMini 保留
.\install.ps1 -Uninstall
```

Windows 也可以从「设置 → 应用 → 已安装的应用」里卸载，条目名 **OpsMini 运维面板**。它调用的是安装时落到安装目录的 `install.ps1 -Uninstall`，行为与上面完全一致；缺少管理员权限时会自动请求提权。

## 兼容性

- Linux：`x86_64` / `aarch64`；无 systemd 的环境用 `-n` 跳过服务注册，改用手动启动
- Windows：Windows 10 1809 / Server 2019 及以上（ConPTY 需要），`amd64` / `arm64`
- Windows 上网站管理、数据库管理、一键部署、系统 crontab 不适用，相关接口返回 501；其余功能（监控、终端、文件、服务、防火墙、安全基线、威胁检测、文件完整性）均可用

## 账号恢复（忘记密码 / 丢失 MFA 验证码）

### Linux

在服务器上执行以下命令，把 `opsmini` 换成实际用户名（先停服务，操作完再启动）：

```bash
# 停服务
systemctl stop opsmini

# 重置密码（会生成并打印一个随机新密码）
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-pass opsmini

# 启动服务
systemctl start opsmini
```

清除 MFA 绑定同理，把 `-reset-pass` 换成 `-reset-mfa`。

### Windows

管理员 PowerShell 执行，路径按实际安装目录调整：

```powershell
# 停服务
Stop-Service OpsMini

# 重置密码（会生成并打印一个随机新密码）
& "C:\Program Files\OpsMini\opsmini.exe" -config "C:\Program Files\OpsMini\config.yaml" -reset-pass opsmini

# 清除 MFA 绑定
& "C:\Program Files\OpsMini\opsmini.exe" -config "C:\Program Files\OpsMini\config.yaml" -reset-mfa opsmini

# 启动服务
Start-Service OpsMini
```

执行后会输出一行 `已重置用户 xxx 的密码为：xxxx`，用该新密码登录面板后，请在「系统管理 - 用户」中自行修改密码；MFA 清除后可在「面板设置 - 双因素验证」重新扫码绑定。

> `-reset-pass` / `-reset-mfa` 是账号恢复子命令，直接操作 SQLite 数据库后即退出，不会启动 Web 服务。务必保证操作时服务处于停止状态，避免数据库写入冲突。

### 初始密码

| 平台 | 位置 |
|---|---|
| Linux | `/data/opsmini/.init_passwd`（600） |
| Windows | `C:\ProgramData\OpsMini\.init_passwd` |

初始账号是 `opsmini`，首次登录后请在面板中修改。

## 与 OpsAnt 侧安装脚本的区别

OpsAnt / 管控平台在「主机管理 → 一键安装」里下发的命令，用的是**另一套脚本**：脚本与安装包都由 OpsAnt 自己托管，安装包从 OpsAnt 下载，并自动写入接入信息。它和本目录的官网脚本是两回事：

| | 本目录脚本（官网） | OpsAnt 侧脚本 |
|---|---|---|
| 位置 | `opsmini/install/`，发布到 `https://opsmini.com/install.sh`、`/install.ps1` | `opsant/install/opsmini/`，运行时投放到 `uploads/agent/opsmini/` |
| 安装包来源 | 阿里云 OSS（`opsmini.oss-cn-beijing.aliyuncs.com`） | OpsAnt 自身（`--base-url` / `-BaseUrl` 指向 `/uploads/agent/opsmini`），目标机无需访问外网 |
| OpsAnt 接入 | 不涉及；接入参数是可选项 | 必填，装完即自动注册 |
| 典型用途 | 单机自建面板，或从官网直接装 | 由 OpsAnt / 管控平台纳管 |

OpsAnt 下发的两条命令长这样（仅供参考，实际命令由平台生成并带复制按钮）：

```bash
# Linux
curl -fsSL http://<opsant>/uploads/agent/opsmini/install.sh | sudo bash -s -- \
  --server-addr <opsant>:4507 --server-token <token> \
  --base-url http://<opsant>/uploads/agent/opsmini --version v1.5.0
```

```powershell
# Windows（管理员 PowerShell）
iwr -useb http://<opsant>/uploads/agent/opsmini/install.ps1 -OutFile $env:TEMP\opsmini-install.ps1
& $env:TEMP\opsmini-install.ps1 -ServerAddr <opsant>:4507 -ServerToken <token> `
  -BaseUrl http://<opsant>/uploads/agent/opsmini -Version v1.5.0
```

**不要混用**：不要把本目录的脚本覆盖到 OpsAnt 的投放目录，也不要拿 OpsAnt 的脚本替换官网发布物。两者的默认下载源、参数集合、接入行为都不同，各自独立维护。
