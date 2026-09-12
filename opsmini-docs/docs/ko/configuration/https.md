# HTTPS와 인증서

## 권장 방안: 리버스 프록시에서 TLS 종료

패널 자체는 HTTP를 리스닝하며, Nginx / Caddy가 에지에서 TLS를 종료합니다:

```nginx
server {
    listen 443 ssl http2;
    server_name panel.example.com;

    ssl_certificate     /etc/ssl/panel.example.com.crt;
    ssl_certificate_key /etc/ssl/panel.example.com.key;

    location / {
        proxy_pass http://127.0.0.1:8888;
        # ... 나머지 proxy_set_header는「포트와 리버스 프록시」참조
    }
}
```

Caddy는 한 줄로 자동 발급 및 갱신이 가능합니다:

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## 인증서 출처

- **Let's Encrypt**: 권장, Caddy / certbot 자동 발급 및 갱신
- **클라우드 제공자 인증서**: 알리바바 클라우드 / 텐센트 클라우드 등 무료 DV 인증서
- **패널 내 웹사이트 모듈**: 패널이 호스팅하는 Nginx 사이트의 SSL 인증서를 관리할 때 사용

## 보안 권장 사항

1. 평문 HTTP로 패널을 노출하지 말고 HTTPS를 강제하세요
2. 보안 진입 프리픽스（패널「설정 → 기본 설정」또는 `server.secret_entry`）와 함께 사용해 진입점을 숨기세요
3. 방화벽으로 8888 포트를 로컬에서만 접근 가능하도록 제한하고, 리버스 프록시의 443만 노출하세요
