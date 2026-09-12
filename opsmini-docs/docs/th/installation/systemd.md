# จัดการด้วย systemd

ลงทะเบียน OpsMini เป็นบริการ systemd เพื่อเริ่มอัตโนมัติตอนบูตและดูแลโปรเซส

## สร้างไฟล์บริการ

สร้าง `/etc/systemd/system/opsmini.service`:

```ini
[Unit]
Description=OpsMini Server Panel
After=network.target

[Service]
Type=simple
ExecStart=/data/opsmini/opsmini -config /data/opsmini/config.yaml
Restart=always
RestartSec=3
# เสริมความปลอดภัย (ทางเลือก)
User=opsmini
Group=opsmini
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

## เปิดใช้และเริ่ม

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /data/opsmini /data/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

## คำสั่งจัดการที่พบบ่อย

```bash
systemctl status opsmini    # ดูสถานะ
systemctl restart opsmini   # รีสตาร์ท
systemctl stop opsmini      # หยุด
journalctl -u opsmini -f    # ดูล็อกแบบเรียลไทม์
```

## สภาพแวดล้อมที่ไม่มี systemd

คอนเทนเนอร์/ระบบ minimal บางตัวไม่มี systemd สามารถเริ่มด้วยตนเองได้:

```bash
nohup /data/opsmini/opsmini -config /data/opsmini/config.yaml > /var/log/opsmini.log 2>&1 &
```
