# 端口与反向代理

面板默认监听 `8888` 端口，生产环境建议通过 Nginx / Caddy 反向代理暴露 80/443。

## Nginx 反向代理

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

        # Web 终端（WebSocket）需要升级支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

> 关键：`proxy_http_version 1.1` + `Upgrade` / `Connection` 头是 Web 终端正常工作的前提。

## Caddy 反向代理（自动 HTTPS）

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## 安全入口

若配置了 `secret_entry: /opsmini_panel`，面板路径变为：

```
http://<host>:8888/opsmini_panel
```

配合反向代理可隐藏真实入口，防端口扫描。反向代理时需将前缀一并转发：

```nginx
location /opsmini_panel/ {
    proxy_pass http://127.0.0.1:8888/opsmini_panel/;
    # ... 其余 proxy_set_header 同上
}
```

## 修改监听端口

编辑 `config.yaml` 的 `server.port` 后重启：

```bash
sudo systemctl restart opsmini
```
