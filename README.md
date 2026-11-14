<div align="center">

# OpsMini

**Linux / Windows 主机上的 AI 运维 Agent**

[English](docs/README.en.md) · [简体中文](README.md) · [繁體中文](docs/README.zh-TW.md) · [日本語](docs/README.ja.md) · [한국어](docs/README.ko.md) · [ไทย](docs/README.th.md) · [Deutsch](docs/README.de.md)

</div>

---

OpsMini 是跑在 **Linux 与 Windows** 主机上的常驻 AI 运维 Agent：自己采集指标、自己判断异常、自己动手处理，遇到高危动作停下来等你点头。Web 界面只是它的观察窗——你可以什么都不点，它照样每天把活干完并留下报告。整个 Agent 是一个几十 MB 的静态二进制，数据全部留在本机。

### 平台支持

| 平台 | 架构 | 交付形态 | 安装方式 |
|---|---|---|---|
| Linux | amd64 / arm64 | 静态二进制 | `curl … \| sudo bash` |
| Windows | amd64 / arm64 | 单 exe（无需运行库） | 管理员 PowerShell 一行命令 |

两个平台功能对齐：指标采集、AI 智能体、定时任务、多机纳管、标准 REST API、Docker 管理。平台差异部分见下文「平台差异」。

### 特性

- **AI 智能体（Agent）** — 自研轻量 ReAct 内核，29+ 内置工具，模型自主查证、逐步定位问题
- **分级授权 + 高危确认** — L0 只读自主执行、L1 可配置、L2 必须人工批准、L3 危险命令硬禁止
- **定时 AI 任务** — 在计划任务里加一条「Agent 任务」，如每天早上自动巡检并生成**单文件 HTML 报告**（可打开可下载；无人值守时自动拒绝高危操作）
- **双向 MCP** — 既可作为 MCP 客户端接入外部工具，也可把自身工具集发布为 MCP Server
- **多机统一管理** — 通过 OpsMini Agent（gRPC，免 SSH）纳管其它主机并远程执行、收发文
- **标准 REST API** — `/api/v1`（浏览器 UI）与 `/mcp`（机器对机器，Model Context Protocol）
- **监控指标导出** — 内置 Prometheus 端点 `/metrics`；Windows 上**指标名与 label 和 windows_exporter 0.32.1 一致**，原有的 Grafana 面板与告警规则可直接复用
- **Docker 管理** — 容器、镜像、卷、网络
- **终端** — 命令行终端（Linux 走 pty，Windows 走 ConPTY）+ **Windows 图形终端**（实时桌面画面，可勾选后接管鼠标键盘）
- **主机安全** — 安全评分、基线检查、登录安全、防火墙、文件完整性、威胁检测，Windows 有独立的一套检查项
- **7 语言 i18n** — 简/繁中文、英文、日文、韩文、泰文、德文
- **单二进制交付** — 前端通过 `go:embed` 嵌入，零运行时依赖，无 CGO

### 平台差异

| 能力 | Linux | Windows |
|---|---|---|
| 服务托管 | systemd | Windows 服务（SCM） |
| 命令行终端 | pty | ConPTY（PowerShell / cmd） |
| 图形终端 | — | GDI 采屏 + 输入注入（需桌面会话） |
| 指标 | 内置 node_exporter 风格 | 内置 windows_exporter 兼容实现 |
| 安全基线 | Linux 加固项 | UAC / 防火墙 / RDP / SMBv1 / Defender 等 8 项 |
| 网站 / 数据库 / 一键部署 | ✅ | 不适用（按平台返回 501） |

### 技术栈

Go · Gin · GORM · SQLite（纯 Go）· Vue 3 · ECharts · MCP Go SDK

### 快速开始

**Linux**

```bash
curl -fsSL https://opsmini.com/install.sh | sudo bash
```

**Windows**（管理员 PowerShell）

```powershell
iwr -useb https://opsmini.com/install.ps1 -OutFile $env:TEMP\opsmini.ps1; & $env:TEMP\opsmini.ps1
```

两种方式都会自动注册为系统服务并设置开机自启；安装完成后打开 `http://<主机IP>:8888`，默认账号 `opsmini`，初始密码见安装输出或数据目录下的 `.init_passwd`。

**接入 OpsAnt（可选）**：安装时带上地址与 token 即可自动注册，不需要装完再进面板配置。

```bash
# Linux
curl -fsSL <opsant>/uploads/agent/opsmini/install.sh | sudo bash -s -- \
  --server-addr <opsant-ip>:4507 --server-token <token> \
  --base-url <opsant>/uploads/agent/opsmini --version v1.5.0
```

```powershell
# Windows
iwr -useb <opsant>/uploads/agent/opsmini/install.ps1 -OutFile $env:TEMP\opsmini.ps1
& $env:TEMP\opsmini.ps1 -ServerAddr <opsant-ip>:4507 -ServerToken <token> `
  -BaseUrl <opsant>/uploads/agent/opsmini -Version v1.5.0
```

**从源码构建**

```bash
make build          # 编译当前平台
make build-all      # 交叉编译 Linux amd64/arm64（macOS 用于开发调试）

./dist/opsmini -config configs/config.yaml
```

Windows 目标（amd64 / arm64）用同一套方法交叉编译：

```bash
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -o dist/opsmini.exe ./cmd/agent
```

### 文档

- [开发者手册](docs/developer-guide.zh-CN.md)
- [后端架构设计](docs/backend-architecture.md)
- [AI 智能体设计 / AI Agent design](docs/ai-agent.md)（7 语言）
- [构建与部署手册](docs/build-and-deploy.md)
- [主机安全设计](docs/host-security-design.md)

### 版权

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
