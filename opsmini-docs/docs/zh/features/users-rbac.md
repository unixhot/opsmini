# 用户与权限

OpsMini 采用基于角色的访问控制（RBAC），精细管理面板操作权限。

## 用户管理

- 创建 / 编辑 / 停用 / 删除用户
- 查看最后登录时间
- 每个用户可绑定独立 MFA（TOTP）
- 个人设置页支持修改昵称、头像、邮箱与密码

## 角色与权限

内置三种角色：

| 角色 | 权限范围 |
|------|----------|
| `admin` | 全部权限（含用户 / 角色 / 设置） |
| `operator` | 日常运维（应用、容器、文件、终端、计划任务等） |
| `readonly` | 只读查看 |

支持自定义角色，按**权限点**精细分配，例如：

- `user.create` / `user.edit` / `user.delete`
- `website.create` / `website.edit` / `website.delete`
- `container.edit` / `container.delete`
- `security.scan` / `security.firewall` / `security.fim` / `security.threat`
- `settings.view` / `settings.edit`（面板设置）
- `apps.install`、`cron.create`、`database.create`、`file.write`、`mcp.manage`、`skill.manage`、`alert.manage` 等

## 双因素验证（2FA） {: #2fa }

- **全局开关**：面板「设置 → 二步验证」开启后，所有已绑定验证器的账号登录时均需输入 6 位动态码
- **每用户绑定**：用户在个人安全设置中生成二维码，用 Google Authenticator / 1Password / 各云厂商验证器 APP 扫码绑定
- **账号恢复**：丢失验证器时，可登录服务器执行 `opsmini -reset-mfa <username>` 重置该用户的 MFA 绑定

## 安全设计

- 密码 bcrypt 哈希存储
- 短期 access JWT（15 分钟）+ 可撤销 refresh token（7 天）
- JWT 签名密钥首次启动自动生成并持久化到数据库，不落配置文件
- 登录失败限流与锁定（防爆破）
- 关键操作写审计日志
- 忘记密码可登录服务器执行 `opsmini -reset-pass <username>` 重置
