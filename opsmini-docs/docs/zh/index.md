---
title: OpsMini — 轻量级 Linux 主机运维面板
hide:
  - navigation
  - toc
---

<div class="home-hero" markdown>

# OpsMini

**轻量级 Linux 主机运维面板** — 内置 AI 大模型运维，标准 REST API 对外集成

对标宝塔 / 1Panel，让单机运维更简单、更智能、更可被集成。

<div class="home-cta" markdown>
[快速安装](getting-started/quick-install.md){ .md-button .md-button--primary }
[查看文档](getting-started/index.md){ .md-button }
[:fontawesome-brands-github: GitHub](https://github.com/unixhot/opsmini){ .md-button }
</div>

</div>

---

## 两大核心差异化

<div class="grid cards" markdown>

-   :material-robot-outline: **AI 大模型运维**

    ---

    自然语言诊断故障、分析日志、执行运维命令。接入 OpenAI / DeepSeek / Qwen / Ollama，让 AI 成为你的运维副驾。

-   :material-api: **标准 REST API**

    ---

    内置 `/api/v1`（面板 API）与 `/agent/v1`（机器对机器 API），任何监控平台、自动化脚本、编排工具都能直接集成调用。

</div>

---

## 特性一览

<div class="grid cards" markdown>

-   :material-view-dashboard-outline: **仪表盘与监控**

    ---

    实时 CPU / 内存 / 磁盘 / 网络时序，告警规则与告警事件闭环。

-   :material-docker: **容器管理**

    ---

    Docker 容器、镜像、卷、网络四类资源全托管，另有软件商店一键安装常用应用。

-   :material-console: **Web 终端**

    ---

    WebSocket + pty 实现的类 SSH 交互终端，浏览器里直接操作服务器。

-   :material-shield-check-outline: **主机安全**

    ---

    基线检查、文件完整性监控（FIM）、威胁检测、防火墙、登录安全，安全态势一屏掌握。

-   :material-folder-outline: **文件与网站**

    ---

    文件浏览 / 上传 / 编辑，Nginx 网站、数据库、SSL 证书一站式管理。

-   :material-translate: **7 语言 · 单二进制**

    ---

    简/繁中文、英、日、韩、泰、德 7 语言界面；前端 `go:embed` 嵌入，约 30MB 静态链接，零运行时依赖。

</div>

---

## 一键安装

<div class="home-section" markdown>

### 5 分钟上手

```bash
# Linux x86_64 / aarch64，一键脚本安装到 /data/opsmini，端口 8888
curl -fsSL https://opsmini.com/install.sh | sudo bash
```

首次启动自动生成管理员账号 `opsmini` 与随机密码（见启动日志），打开 `http://<host>:8888` 即可。

[查看完整安装文档 →](installation/index.md){ .md-button }

</div>

---

<div class="home-section" markdown>

## 立即开始

一行命令部署，AI 赋能运维，标准 API 打通集成。

[开始安装](getting-started/quick-install.md){ .md-button .md-button--primary }
[探索文档](getting-started/index.md){ .md-button }

</div>
