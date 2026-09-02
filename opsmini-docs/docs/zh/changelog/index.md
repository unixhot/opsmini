# 更新日志

各版本变更记录。

## v1.0.25（当前）

近期主要更新：

- **配置精简**：JWT 签名密钥首次启动自动生成并持久化到数据库，不再手动配置、不落配置文件
- **AI 配置面板化**：AI 大模型接入（模型 / API Key / Base URL / 开关）迁移到面板「设置 → AI 大模型接入」页
- **Agent Token 面板化**：`/agent/v1` 认证令牌迁移到面板「设置 → API Token」页，留空禁用
- **新增双因素验证（2FA）**：全局开关 + 每用户 TOTP 绑定，支持 `-reset-mfa` 账号恢复
- **新增账号恢复**：`-reset-pass` 重置用户密码为随机强密码
- **监控导出**：Prometheus `/metrics` 认证改为 HTTP Basic auth，支持面板设置覆盖
- **主机安全模块**：基线检查、文件完整性监控（FIM）、威胁检测、防火墙、登录安全
- **软件商店**：应用分类 + 一键安装 + 官方模版同步
- **通知管理**：SMTP / SNMP 通知渠道
- **AI 技能（Skill Hub）**：技能检索、安装、上传
- **日志保留**：审计 / 访问日志保留天数可配置
- **7 语言 i18n** 全覆盖，品牌视觉升级（蓝色 logo、侧边栏配色）

## v1.0.0

- 首个可运行的最小面板：登录 / 用户 / RBAC / 仪表盘 / 监控
- 单二进制交付，前端 `go:embed` 嵌入
- 双 API：`/api/v1` 与 `/agent/v1`

> 完整版本历史以仓库 Release 为准：[GitHub Releases](https://github.com/unixhot/opsmini/releases)
