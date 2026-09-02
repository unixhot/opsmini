# Ports & Reverse Proxy

The panel listens on port `8888` by default. Expose it via Nginx / Caddy on 80/443 in production.

## Nginx Reverse Proxy

```nginx
server {
    listen 80;
    server_name panel.example.com;

    location / {
        proxy_pass http://127.0.0.1:8888;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket upgrade for the web terminal
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

> The `proxy_http_version 1.1` + `Upgrade`/`Connection` headers are required for the web terminal.

## Caddy Reverse Proxy (Automatic HTTPS)

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## Secret Entry

If `secret_entry: /opsmini_panel` is set, the panel path becomes:

```
http://<host>:8888/opsmini_panel
```

Forward the prefix in the reverse proxy:

```nginx
location /opsmini_panel/ {
    proxy_pass http://127.0.0.1:8888/opsmini_panel/;
    # ... same proxy_set_header as above
}
```

## Change the Listen Port

Edit `server.port` in `config.yaml`, then restart:

```bash
sudo systemctl restart opsmini
```
