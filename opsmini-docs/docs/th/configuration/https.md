# HTTPS และใบรับรอง

## วิธีแนะนำ: Reverse Proxy ยุติ TLS

ตัวพาเนล listen ด้วย HTTP โดยให้ Nginx / Caddy ยุติ TLS ที่ขอบ:

```nginx
server {
    listen 443 ssl http2;
    server_name panel.example.com;

    ssl_certificate     /etc/ssl/panel.example.com.crt;
    ssl_certificate_key /etc/ssl/panel.example.com.key;

    location / {
        proxy_pass http://127.0.0.1:8888;
        # ... proxy_set_header อื่น ๆ ดูที่ "พอร์ตและ Reverse Proxy"
    }
}
```

Caddy เพียงบรรทัดเดียวสามารถออกและต่ออายุได้อัตโนมัติ:

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

## แหล่งที่มาของใบรับรอง

- **Let's Encrypt**: แนะนำ Caddy / certbot ออกและต่ออายุอัตโนมัติ
- **ใบรับรองผู้ให้บริการคลาวด์**: ใบรับรอง DV ฟรี เช่น Alibaba Cloud / Tencent Cloud
- **โมดูลเว็บไซต์ในพาเนล**: ใช้เมื่อจัดการใบรับรอง SSL ของไซต์ Nginx ที่พาเนลดูแล

## คำแนะนำด้านความปลอดภัย

1. ห้ามเปิดพาเนลด้วย HTTP ข้อความล้วน บังคับใช้ HTTPS
2. ใช้ร่วมกับคำนำหน้าจุดเข้าความปลอดภัย ("การตั้งค่า → การตั้งค่าพื้นฐาน" ของพาเนล หรือ `server.secret_entry`) เพื่อซ่อนจุดเข้า
3. ใช้ไฟร์วอลล์จำกัดพอร์ต 8888 ให้เข้าถึงได้เฉพาะเครื่องตัวเอง เปิดเฉพาะพอร์ต 443 ของ reverse proxy
