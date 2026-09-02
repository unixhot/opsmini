# HTTPS & Certificates

## Recommended: Terminate TLS at the Reverse Proxy

The panel serves HTTP; terminate TLS at Nginx / Caddy:

```nginx
server {
    listen 443 ssl http2;
    server_name panel.example.com;

    ssl_certificate     /etc/ssl/panel.example.com.crt;
    ssl_certificate_key /etc/ssl/panel.example.com.key;

    location / {
        proxy_pass http://127.0.0.1:8888;
        # ... proxy_set_header as in "Ports & Reverse Proxy"
    }
}
```

Caddy auto-issues and renews:

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## Certificate Sources

- **Let's Encrypt**: recommended, via Caddy / certbot
- **Cloud provider**: free DV certificates (Alibaba Cloud / Tencent Cloud, etc.)
- **Panel website module**: for SSL of Nginx sites managed by the panel

## Security Tips

1. Never expose the panel over plain HTTP; enforce HTTPS.
2. Use `server.secret_entry` to hide the entry point.
3. Restrict port 8888 to localhost and expose only 443.
