# คำถามที่พบบ่อย

## การติดตั้งและการเริ่ม

**Q: `go mod tidy` ค้างหรือแจ้ง Bad Gateway?**

ในเครือข่ายภายในประเทศจีน `proxy.golang.org` อาจถูกบล็อก ให้รัน `export GOPROXY=https://goproxy.cn,direct`

**Q: แจ้งว่าเวอร์ชัน Go ต่ำเกินไป?**

OpsMini ต้องการ Go 1.25+ ดาวน์โหลดไบนารี precompiled ทางการและเพิ่มใน PATH

**Q: เข้าถึง `/` แล้วได้ 301 หรือหน้าว่าง?**

ทรัพยากร static ของ frontend ถูกฝังผ่าน `go:embed` แล้ว กรุณาใช้ artifact จาก `make build` อย่าแยกไดเรกทอรี frontend เอง

## บัญชีและความปลอดภัย

**Q: ลืมรหัสผ่านผู้ดูแล?**

รันบนเซิร์ฟเวอร์ `opsmini -config <path> -reset-pass opsmini` เพื่อรีเซ็ตเป็นรหัสผ่านสุ่ม (ดูรายละเอียดที่[การติดตั้งและการ deploy](../installation/index.md))

**Q: ทำรหัสยืนยัน MFA สองปัจจัยหาย?**

รัน `opsmini -config <path> -reset-mfa opsmini` เพื่อล้างการผูก MFA แล้วล็อกอินใหม่เพื่อผูกอีกครั้ง

## การ deploy และการปฏิบัติการ

**Q: อัปเกรดเวอร์ชันอย่างไร?**

สำรอง `opsmini.db` → แทนที่ไบนารี → รีสตาร์ทบริการ SQLite ถูก GORM auto-migrate โดยปกติไม่ต้องแก้ตารางเอง

**Q: เว็บเทอร์มินัลเชื่อมต่อไม่ได้หลัง reverse proxy?**

ต้องเปิดใช้ส่วนหัวอัปเกรด WebSocket ใน Nginx (`proxy_http_version 1.1` + `Upgrade`/`Connection`) ดูรายละเอียดที่[พอร์ตและ Reverse Proxy](../configuration/ports-proxy.md)

**Q: ให้ Prometheus มอนิเตอร์เครื่องนี้อย่างไร?**

เปิดใช้ `metrics` ในการกำหนดค่า Prometheus scrape `http://<host>:8888/metrics` โดยตรง
