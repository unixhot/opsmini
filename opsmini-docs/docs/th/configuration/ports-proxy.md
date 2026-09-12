# พอร์ตและ Reverse Proxy

พาเนล listen พอร์ต `8888` เป็นค่าเริ่มต้น ในสภาพแวดล้อม production แนะนำให้เปิดพอร์ต 80/443 ผ่าน Nginx / Caddy reverse proxy

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

        # เว็บเทอร์มินัล (WebSocket) ต้องเปิดใช้การอัปเกรด
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

> สำคัญ: `proxy_http_version 1.1` + ส่วนหัว `Upgrade` / `Connection` เป็นเงื่อนไขที่เว็บเทอร์มินัลทำงานได้ตามปกติ

## Caddy Reverse Proxy (HTTPS อัตโนมัติ)

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## จุดเข้าความปลอดภัย

หากกำหนดค่า `secret_entry: /opsmini_panel` พาธของพาเนลจะเปลี่ยนเป็น:

```
http://<host>:8888/opsmini_panel
```

ใช้ร่วมกับ reverse proxy เพื่อซ่อนจุดเข้าจริง ป้องกันการสแกนพอร์ต เมื่อใช้ reverse proxy ต้องส่งต่อคำนำหน้าไปด้วย:

```nginx
location /opsmini_panel/ {
    proxy_pass http://127.0.0.1:8888/opsmini_panel/;
    # ... proxy_set_header อื่น ๆ เหมือนด้านบน
}
```

## การเปลี่ยนพอร์ต listen

แก้ไข `server.port` ใน `config.yaml` แล้วรีสตาร์ท:

```bash
sudo systemctl restart opsmini
```
