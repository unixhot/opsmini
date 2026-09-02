# 埠與反向代理

面板預設監聽 `8888` 埠，生產環境建議透過 Nginx / Caddy 反向代理暴露 80/443。

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

> 關鍵：`proxy_http_version 1.1` + `Upgrade` / `Connection` 頭是 Web 終端正常工作的前提。

## Caddy 反向代理（自動 HTTPS）

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## 安全入口

若配置了 `secret_entry: /opsmini_panel`，面板路徑變為：

```
http://<host>:8888/opsmini_panel
```

配合反向代理可隱藏真實入口，防埠掃描。反向代理時需將字首一併轉發：

```nginx
location /opsmini_panel/ {
    proxy_pass http://127.0.0.1:8888/opsmini_panel/;
    # ... 其余 proxy_set_header 同上
}
```

## 修改監聽埠

編輯 `config.yaml` 的 `server.port` 後重啟：

```bash
sudo systemctl restart opsmini
```
