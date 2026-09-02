# HTTPS 与证书

## 推荐方案：反向代理终止 TLS

面板本身监听 HTTP，由 Nginx / Caddy 在边缘终止 TLS：

```nginx
server {
    listen 443 ssl http2;
    server_name panel.example.com;

    ssl_certificate     /etc/ssl/panel.example.com.crt;
    ssl_certificate_key /etc/ssl/panel.example.com.key;

    location / {
        proxy_pass http://127.0.0.1:8888;
        # ... 其余 proxy_set_header 见「端口与反向代理」
    }
}
```

Caddy 一行即可自动签发并续期：

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## 证书来源

- **Let's Encrypt**：推荐，Caddy / certbot 自动签发与续期
- **云厂商证书**：阿里云 / 腾讯云等免费 DV 证书
- **面板内网站模块**：管理面板托管的 Nginx 站点 SSL 证书时使用

## 安全建议

1. 禁止明文 HTTP 暴露面板，强制 HTTPS
2. 配合安全入口前缀（面板「设置 → 基础设置」或 `server.secret_entry`）隐藏入口
3. 通过防火墙限制 8888 端口仅本机可访问，仅暴露反代的 443
