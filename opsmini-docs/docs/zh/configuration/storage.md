# 数据存储

OpsMini 使用嵌入式 SQLite 存储全部业务数据（用户、任务、告警、网站、审计日志等）。

## 数据文件

- 默认路径：一键安装为 `/data/opsmini/opsmini.db`（由 `config.yaml` 中 `database.path` 指定，默认相对配置文件同目录）
- 首次启动自动创建并建表（GORM 自动迁移）
- 敏感字段（密钥 / Token / TOTP secret）使用 AES-GCM 加密后落库

## 备份

SQLite 是单文件，直接拷贝即可备份：

```bash
# 建议先停服务再备份，保证一致性
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /backup/opsmini.db.$(date +%s)
sudo systemctl start opsmini
```

## 恢复

```bash
sudo systemctl stop opsmini
cp /backup/opsmini.db.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```

## 存储内容概览

| 数据 | 表 | 说明 |
|------|-----|------|
| 用户与角色 | `users` / `roles` | 账号、密码哈希、RBAC 角色与权限 |
| 会话 | `sessions` | refresh token 与有效期 |
| 面板配置 | `settings` | 主题 / 语言 / 菜单显隐等 KV |
| 网站 / 数据库 | `websites` / `databases` | 站点与数据库实例记录 |
| 计划任务 | `cron_jobs` | 面板计划任务 |
| 告警规则 / 事件 | `alert_rules` / `alert_events` | 监控告警 |
| 审计 / 访问日志 | `audit_logs` / `access_logs` | 操作与访问记录（默认保留 7 天） |

## 数据清理

- 审计日志与访问日志默认保留 7 天，每小时自动清理，可在面板设置中调整保留天数
- 告警事件按保留时间自动清理（默认 30 天）
