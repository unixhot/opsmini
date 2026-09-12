# 포트와 리버스 프록시

패널은 기본적으로 `8888` 포트를 리스닝하며, 프로덕션 환경에서는 Nginx / Caddy 리버스 프록시를 통해 80/443을 노출하는 것을 권장합니다.

## Nginx 리버스 프록시

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

        # 웹 터미널（WebSocket）은 업그레이드 지원이 필요합니다
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

> 핵심: `proxy_http_version 1.1` + `Upgrade` / `Connection` 헤더는 웹 터미널이 정상 동작하기 위한 전제 조건입니다.

## Caddy 리버스 프록시（자동 HTTPS）

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## 보안 진입

`secret_entry: /opsmini_panel`을 설정하면 패널 경로는 다음과 같이 변경됩니다:

```
http://<host>:8888/opsmini_panel
```

리버스 프록시와 함께 사용하면 실제 진입점을 숨겨 포트 스캔을 방지할 수 있습니다. 리버스 프록시 시 프리픽스를 함께 포워딩해야 합니다:

```nginx
location /opsmini_panel/ {
    proxy_pass http://127.0.0.1:8888/opsmini_panel/;
    # ... 나머지 proxy_set_header는 위와 동일
}
```

## 리스닝 포트 변경

`config.yaml`의 `server.port`를 편집한 후 재시작합니다:

```bash
sudo systemctl restart opsmini
```
