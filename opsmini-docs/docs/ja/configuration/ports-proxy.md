# ポートとリバースプロキシ

パネルはデフォルトで `8888` ポートをリッスンします。本番環境では Nginx / Caddy のリバースプロキシ経由で 80/443 を公開することを推奨します。

## Nginx リバースプロキシ

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

        # Web ターミナル（WebSocket）にはアップグレード対応が必要
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

> 重要：`proxy_http_version 1.1` + `Upgrade` / `Connection` ヘッダーが Web ターミナル正常動作の前提です。

## Caddy リバースプロキシ（HTTPS 自動化）

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## セキュリティ入口

`secret_entry: /opsmini_panel` を設定した場合、パネルのパスは次のようになります：

```
http://<host>:8888/opsmini_panel
```

リバースプロキシと組み合わせて実際の入口を隠し、ポートスキャンを防げます。リバースプロキシ時はプレフィックスも一緒に転送する必要があります：

```nginx
location /opsmini_panel/ {
    proxy_pass http://127.0.0.1:8888/opsmini_panel/;
    # ... その他の proxy_set_header は同上
}
```

## リッスンポートの変更

`config.yaml` の `server.port` を編集して再起動します：

```bash
sudo systemctl restart opsmini
```
