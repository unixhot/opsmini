<div align="center">

# OpsMini

**主机上的 AI 运维 Agent**

[English](docs/README.en.md) · [简体中文](README.md) · [繁體中文](docs/README.zh-TW.md) · [日本語](docs/README.ja.md) · [한국어](docs/README.ko.md) · [ไทย](docs/README.th.md) · [Deutsch](docs/README.de.md)

</div>

---

OpsMini 是一台 Linux 主机上的常驻 AI 运维 Agent：自己采集指标、自己判断异常、自己动手处理，遇到高危动作停下来等你点头。Web 界面只是它的观察窗——你可以什么都不点，它照样每天把活干完并留下报告。整个 Agent 是一个几十 MB 的静态二进制，数据全部留在本机。

### 特性

- **AI 智能体（Agent）** — 自研轻量 ReAct 内核，29+ 内置工具，模型自主查证、逐步定位问题
- **分级授权 + 高危确认** — L0 只读自主执行、L1 可配置、L2 必须人工批准、L3 危险命令硬禁止
- **定时 AI 任务** — 在计划任务里加一条「Agent 任务」，如每天早上自动巡检并生成**单文件 HTML 报告**（可打开可下载；无人值守时自动拒绝高危操作）
- **双向 MCP** — 既可作为 MCP 客户端接入外部工具，也可把自身工具集发布为 MCP Server
- **多机统一管理** — 通过 OpsMini Agent（gRPC，免 SSH）纳管其它主机并远程执行、收发文
- **标准 REST API** — `/api/v1`（浏览器 UI）与 `/mcp`（机器对机器，Model Context Protocol）
- **Docker 管理** — 容器、镜像、卷、网络
- **Web 终端** — WebSocket + pty 实现的类 SSH 交互式终端
- **7 语言 i18n** — 简/繁中文、英文、日文、韩文、泰文、德文
- **单二进制交付** — 前端通过 `go:embed` 嵌入，零运行时依赖，无 CGO

### 技术栈

Go · Gin · GORM · SQLite（纯 Go）· Vue 3 · ECharts · MCP Go SDK

### 快速开始

```bash
make build          # 编译当前平台
make build-all      # 交叉编译 Linux amd64/arm64

./dist/opsmini -config configs/config.yaml
# 打开 http://localhost:8888（默认账号 opsmini，密码见首次启动日志）
```

### 文档

- [开发者手册](docs/developer-guide.zh-CN.md)
- [后端架构设计](docs/backend-architecture.md)
- [AI 智能体设计 / AI Agent design](docs/ai-agent.md)（7 语言）
- [构建与部署手册](docs/build-and-deploy.md)
- [主机安全设计](docs/host-security-design.md)

### 版权

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
