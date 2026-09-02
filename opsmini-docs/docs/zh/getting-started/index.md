# 快速开始

欢迎使用 OpsMini —— 一款轻量级 Linux 主机运维面板。本指南帮助你在最短时间内完成安装、登录并了解核心功能。

## 从这里开始

<div class="grid cards" markdown>

-   :material-rocket-launch: **[5 分钟快速安装](quick-install.md)**

    ---

    一行命令完成部署，首次启动自动生成管理员账号与随机密码。

-   :material-login: **[首次登录](first-steps.md)**

    ---

    登录面板、修改密码、绑定双因素验证（MFA）。

-   :material-information-outline: **[产品介绍](introduction.md)**

    ---

    了解 OpsMini 的定位、核心特性与技术栈。

</div>

## 核心概念

| 概念 | 说明 |
|------|------|
| **单机面板** | 每台主机独立运行一个 OpsMini 实例，不依赖中心节点 |
| **单二进制交付** | 前端 UI 通过 `go:embed` 嵌入，部署即拷贝一个可执行文件 |
| **双 API** | `/api/v1` 面向浏览器 UI（JWT + RBAC），`/agent/v1` 面向机器/第三方集成（Token + 命令白名单） |
| **AI 大模型运维** | 自然语言诊断、日志分析、命令执行，接入 OpenAI / DeepSeek / Qwen / Ollama |

## 下一步

完成安装后，建议按顺序阅读：

1. [安装部署](../installation/index.md) — 系统要求、一键安装、手动部署、反向代理
2. [配置指南](../configuration/index.md) — 配置文件、数据存储、HTTPS
3. [功能指南](../features/index.md) — 仪表盘、AI 助手、应用管理、主机安全等
4. [REST API](../api/index.md) — 对外集成接口
