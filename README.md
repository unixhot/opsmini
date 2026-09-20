<div align="center">

# OpsMini

**AI 驱动的主机运维面板**

[English](docs/README.en.md) · [简体中文](README.md) · [繁體中文](docs/README.zh-TW.md) · [日本語](docs/README.ja.md) · [한국어](docs/README.ko.md) · [ไทย](docs/README.th.md) · [Deutsch](docs/README.de.md)

</div>

---

OpsMini 是一款 AI 驱动的主机运维面板（对标宝塔 / 1Panel），以内置 AI 大模型运维与对外标准 REST API 为核心差异化。

### 特性

- **AI 大模型运维** — 自然语言诊断、日志分析、命令执行
- **标准 REST API** — `/api/v1`（浏览器 UI）与 `/agent/v1`（机器对机器）
- **Docker 管理** — 容器、镜像、卷、网络
- **Web 终端** — WebSocket + pty 实现的类 SSH 交互式终端
- **7 语言 i18n** — 简/繁中文、英文、日文、韩文、泰文、德文
- **单二进制交付** — 前端通过 `go:embed` 嵌入，零运行时依赖

### 技术栈

Go · Gin · GORM · SQLite（纯 Go）· Vue 3 · ECharts

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
- [构建与部署手册](docs/build-and-deploy.md)
- [主机安全设计](docs/security-audit.md)

### 版权

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
