# 首次登录与初始化

完成安装后，按以下步骤完成首次登录与安全初始化。

## 1. 获取初始密码

启动日志会打印初始账号信息：

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

密码也可在安装目录的 `/data/opsmini/.init_passwd`（权限 600）中查看。

## 2. 登录面板

1. 浏览器打开 `http://<host>:8888`
2. 输入用户名 `opsmini` 与初始密码
3. 登录成功后进入仪表盘

> ⚠️ 生产环境务必通过 [反向代理 + HTTPS](../configuration/https.md) 暴露面板，避免明文传输。

## 3. 修改密码

1. 进入「个人设置」
2. 在「修改密码」中设置新密码并保存

忘记密码时，可登录服务器执行 `opsmini -reset-pass <username>` 重置。

## 4. 绑定双因素验证（推荐）

1. 在面板「设置 → 二步验证」开启全局 2FA
2. 进入个人安全设置，扫码绑定 TOTP 验证器（Google Authenticator / 1Password 等）
3. 输入动态码完成绑定

绑定后，每次登录需额外输入 6 位动态码，大幅提升账号安全性。丢失验证器可用 `opsmini -reset-mfa <username>` 恢复。

## 5. 配置安全入口（可选）

在面板「设置 → 基础设置」中设置安全入口前缀（或直接改 `config.yaml` 的 `server.secret_entry`）：

```yaml
server:
  secret_entry: "/opsmini_panel"
```

配置后面板地址变为 `http://<host>:8888/opsmini_panel`，可配合反向代理隐藏真实入口，防端口扫描。

## 6. 下一步

- [配置 AI 助手](../configuration/panel-settings.md#ai-config)
- [启用对外 REST API](../api/agent-api.md)
- [探索功能](../features/index.md)
