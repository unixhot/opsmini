# HTTPS & Zertifikate

## Empfohlene Lösung: TLS-Terminierung am Reverse-Proxy

Das Panel selbst lauscht auf HTTP; Nginx / Caddy terminieren TLS am Rand:

```nginx
server {
    listen 443 ssl http2;
    server_name panel.example.com;

    ssl_certificate     /etc/ssl/panel.example.com.crt;
    ssl_certificate_key /etc/ssl/panel.example.com.key;

    location / {
        proxy_pass http://127.0.0.1:8888;
        # ... übrige proxy_set_header siehe „Ports & Reverse-Proxy"
    }
}
```

Mit Caddy genügt eine Zeile für automatische Ausstellung und Verlängerung:

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## Zertifikatsquellen

- **Let's Encrypt**: empfohlen, automatische Ausstellung und Verlängerung durch Caddy / certbot
- **Cloud-Anbieter-Zertifikate**: kostenlose DV-Zertifikate von Alibaba Cloud / Tencent Cloud u. a.
- **Website-Modul im Panel**: für die Verwaltung von SSL-Zertifikaten der vom Panel verwalteten Nginx-Sites

## Sicherheitsempfehlungen

1. Kein unverschlüsseltes HTTP für das Panel, HTTPS erzwingen
2. In Kombination mit dem Präfix des Sicherheitseingangs (Panel „Einstellungen → Grundeinstellungen" oder `server.secret_entry`) den Eingang verbergen
3. Per Firewall Port 8888 nur auf den lokalen Rechner beschränken und nur Port 443 des Reverse-Proxy freigeben
