# 更新日志

各版本变更记录。

## v1.1.0（当前）

**多机管理（Agent / Server 架构，去 SaltStack 化）**

- **Agent gRPC 客户端**：Agent 主动连 OpsMini Server（单端口 gRPC 双向流），心跳 + 指数退避重连，接收执行 / 文件任务并回传结果
- **OpsMini Server 端**：注册 / 心跳 / 连接管理、执行任务下发、结果接收（代码迁至 OpsAnt 项目独立 Go 子项目 `opsmini-server/`，OpsMini 仅保留 Agent）
- **命令 / 脚本执行**：command 与 shell / python 脚本，结果（stdout / stderr / exit code / 耗时）回传
- **文件拉取（PullFile）**：Server 从 Agent 拉取文件（分片上传 + size / md5 校验）
- **文件下发（PushFile）**：Server 向 Agent 下发文件（分片 + 断点续传 + md5 校验），支持目标为目录（保留原文件名）
- **连接状态检测 + 长连接保活**：Agent 在线 / 离线状态、心跳超时判定、gRPC keepalive
- **接入 OpsAnt**：面板「设置」页可视化配置 OpsAnt 地址 / Token（运行时热切换），SSO 免认证跳转
- **移除 `/agent/v1` REST API**：改用 gRPC 双向流（原 REST API 及前端「API Token」设置页一并移除）

**面板功能增强**

- **文件页目录下载**：目录一键压缩为 zip 下载（弹窗提示 + 目录大小 + 所在分区磁盘使用率）
- **文件页目录删除**：递归删除目录（危险操作弹窗 + 输入目录名二次确认）
- **Prometheus 指标增强**：新增 `opsmini_cpu_usage_percent` / `opsmini_mem_usage_percent` 即时使用率、`node_procs_total` 总进程数
- **统一确认弹窗**：所有删除 / 危险操作改用平台自带确认弹窗（替换浏览器原生 confirm）

## v1.0.0（首个正式发布）

OpsMini 首个正式版本，单二进制交付，核心能力：

- **AI 运维**：自然语言诊断、日志智能分析、命令生成与执行（SSE 流式输出）、MCP 工具接入
- **监控告警**：CPU / 内存 / 磁盘 / 网络指标时序采集，告警规则支持邮件、面板、企业微信、钉钉、飞书群机器人通知
- **主机安全**：安全评分、基线检查、文件完整性监控（FIM）、威胁检测、防火墙、登录安全
- **软件商店**：应用分类 + 一键安装 + 官方模版同步
- **AI 技能（Skill Hub）**：技能检索、安装、上传
- **Web 终端**：WebSocket + pty 交互式 shell，多会话
- **常规运维**：网站、数据库、计划任务、文件管理
- **双因素验证（2FA）**：全局开关 + 每用户 TOTP 绑定
- **监控导出**：Prometheus `/metrics` 端点（node_exporter 兼容）
- **通知管理**：SMTP / 企业微信 / 钉钉 / 飞书
- **配置精简**：JWT 密钥、Agent Token、AI 配置均面板化，配置文件仅保留最小项
- **日志系统**：日志级别可配置（info / warn / error），日志落盘
- **账号恢复**：`-reset-pass` / `-reset-mfa` 子命令
- **7 语言 i18n**：简中 / 繁中 / 英 / 日 / 韩 / 泰 / 德
- **白色极简 UI**：白色 + 黑色字体配色，品牌蓝点缀
- **双 API**：`/api/v1`（浏览器用）+ `/agent/v1`（机器间标准 REST）

> 完整版本历史以仓库 Release 为准：[GitHub Releases](https://github.com/unixhot/opsmini/releases)
