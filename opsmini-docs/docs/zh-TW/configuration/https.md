# HTTPS 與證書

## 推薦方案：反向代理終止 TLS

面板本身監聽 HTTP，由 Nginx / Caddy 在邊緣終止 TLS：

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

Caddy 一行即可自動簽發並續期：

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## 證書來源

- **Let's Encrypt**：推薦，Caddy / certbot 自動簽發與續期
- **雲廠商證書**：阿里雲 / 騰訊雲等免費 DV 證書
- **面板內網站模組**：管理面板託管的 Nginx 站點 SSL 證書時使用

## 安全建議

1. 禁止明文 HTTP 暴露面板，強制 HTTPS
2. 配合安全入口字首（面板「設定 → 基礎設定」或 `server.secret_entry`）隱藏入口
3. 透過防火牆限制 8888 埠僅本機可訪問，僅暴露反代的 443
