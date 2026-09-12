# HTTPS と証明書

## 推奨方式：リバースプロキシによる TLS 終端

パネル自体は HTTP でリッスンし、Nginx / Caddy がエッジで TLS を終端します：

```nginx
server {
    listen 443 ssl http2;
    server_name panel.example.com;

    ssl_certificate     /etc/ssl/panel.example.com.crt;
    ssl_certificate_key /etc/ssl/panel.example.com.key;

    location / {
        proxy_pass http://127.0.0.1:8888;
        # ... その他の proxy_set_header は「ポートとリバースプロキシ」を参照
    }
}
```

Caddy なら 1 行で自動発行と更新ができます：

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## 証明書の入手先

- **Let's Encrypt**：推奨。Caddy / certbot で自動発行と更新
- **クラウド事業者の証明書**：Alibaba Cloud / Tencent Cloud などの無料 DV 証明書
- **パネル内ウェブサイトモジュール**：パネルが管理する Nginx サイトの SSL 証明書を管理する場合に使用

## セキュリティ推奨事項

1. 平文 HTTP でのパネル公開を禁止し、HTTPS を強制する
2. セキュリティ入口プレフィックス（パネル「設定 → 基本設定」または `server.secret_entry`）と組み合わせて入口を隠す
3. ファイアウォールで 8888 ポートをローカルのみに制限し、リバースプロキシの 443 のみ公開する
