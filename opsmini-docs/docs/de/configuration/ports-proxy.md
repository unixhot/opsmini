# Ports & Reverse-Proxy

Das Panel lauscht standardmäßig auf Port `8888`. In der Produktionsumgebung empfiehlt es sich, 80/443 über einen Reverse-Proxy mit Nginx / Caddy bereitzustellen.

## Nginx-Reverse-Proxy

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

        # Das Web-Terminal (WebSocket) benötigt Upgrade-Unterstützung
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

> Entscheidend: `proxy_http_version 1.1` plus die Header `Upgrade` / `Connection` sind die Voraussetzung dafür, dass das Web-Terminal korrekt funktioniert.

## Caddy-Reverse-Proxy (automatisches HTTPS)

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## Sicherheitseingang

Wenn `secret_entry: /opsmini_panel` konfiguriert ist, lautet der Panel-Pfad:

```
http://<host>:8888/opsmini_panel
```

In Kombination mit einem Reverse-Proxy lässt sich der tatsächliche Eingang verbergen und Port-Scans vorbeugen. Beim Reverse-Proxy muss das Präfix mit weitergeleitet werden:

```nginx
location /opsmini_panel/ {
    proxy_pass http://127.0.0.1:8888/opsmini_panel/;
    # ... übrige proxy_set_header wie oben
}
```

## Lauschport ändern

`server.port` in `config.yaml` bearbeiten und neu starten:

```bash
sudo systemctl restart opsmini
```
