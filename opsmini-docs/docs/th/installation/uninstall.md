# ถอนการติดตั้ง

## หยุดและลบบริการ

```bash
# หยุดและปิดใช้บริการ systemd
sudo systemctl stop opsmini
sudo systemctl disable opsmini
sudo rm -f /etc/systemd/system/opsmini.service
sudo systemctl daemon-reload
```

## ลบไบนารีและการกำหนดค่า

```bash
# ลบไดเรกทอรีติดตั้ง (ค่าเริ่มต้น /data/opsmini หรือไดเรกทอรีที่คุณกำหนดเอง)
sudo rm -rf /data/opsmini
# หรือ
sudo rm -rf /data/opsmini /data/opsmini
```

## ลบผู้ใช้เฉพาะ (ทางเลือก)

```bash
sudo userdel opsmini
```

## หมายเหตุ

- การถอนการติดตั้งไม่กระทบทรัพยากรที่พาเนลจัดการ เช่น Docker คอนเทนเนอร์ / อิมเมจ, ไซต์ Nginx, ฐานข้อมูล บนโฮสต์ ทรัพยากรเหล่านี้ต้องจัดการแยกต่างหาก
- หากต้องการล้างทั้งหมด กรุณายืนยันว่า `/data/opsmini/opsmini.db` ไม่มีข้อมูลธุรกิจที่ต้องเก็บรักษา
