# 常见问题

## 安装与启动

**Q：`go mod tidy` 卡住或报 Bad Gateway？**

国内网络下 `proxy.golang.org` 可能被墙，执行 `export GOPROXY=https://goproxy.cn,direct`。

**Q：提示 Go 版本过低？**

OpsMini 要求 Go 1.25+。下载官方预编译二进制并加入 PATH。

**Q：访问 `/` 返回 301 或空白？**

前端静态资源已通过 `go:embed` 嵌入，请使用 `make build` 产物，勿手动拆分前端目录。

## 账号与安全

**Q：忘记管理员密码？**

服务器上执行 `opsmini -config <path> -reset-pass opsmini` 重置为随机密码（详见[安装部署](../installation/index.md)）。

**Q：丢失 MFA 双因素验证码？**

执行 `opsmini -config <path> -reset-mfa opsmini` 清除 MFA 绑定后重新登录绑定。

## 部署与运维

**Q：如何升级版本？**

备份 `opsmini.db` → 替换二进制 → 重启服务。SQLite 由 GORM 自动迁移，通常无需手动改表。

**Q：Web 终端在反向代理后连不上？**

需在 Nginx 中启用 WebSocket 升级头（`proxy_http_version 1.1` + `Upgrade`/`Connection`），详见[端口与反向代理](../configuration/ports-proxy.md)。

**Q：如何让 Prometheus 监控本机？**

在配置中启用 `metrics`，Prometheus 直接 scrape `http://<host>:8888/metrics`。
