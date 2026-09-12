# Agent API (/agent/v1)

Agent API เป็นอินเทอร์เฟซ REST มาตรฐานสำหรับ **เครื่อง / ระบบของบุคคลที่สาม** สำหรับแพลตฟอร์มมอนิเตอร์ สคริปต์อัตโนมัติ และเครื่องมือออร์เคสเตรชันเพื่อผสานรวมและเรียกใช้งาน

## ความแตกต่างจาก Panel API

| มิติ | Panel API `/api/v1` | Agent API `/agent/v1` |
|------|-------------------|----------------------|
| ผู้ใช้งาน | เบราว์เซอร์ (คน) | ระบบภายนอก (เครื่อง) |
| การยืนยันตัวตน | เซสชันผู้ใช้ JWT + RBAC | Agent Token (Bearer) |
| ขอบเขต | ฟังก์ชัน UI ทั้งหมด + เทอร์มินัล WS | การสอบถามทรัพยากร + การรันคำสั่ง (ไม่มี UI / เทอร์มินัล) |
| การออกแบบ | เน้นการโต้ตอบ | เน้นระบบอัตโนมัติ (idempotent, retry ได้) |

## การยืนยันตัวตน

1. สร้าง access token ในหน้า "การตั้งค่า → API Token" ของพาเนล (token จะถูกเก็บถาวรในฐานข้อมูล ไม่ได้เขียนใน `config.yaml` อีกต่อไป)
2. คำขอจะต้องแนบ `Authorization: Bearer <token>`
3. มิดเดิลแวร์จะตรวจสอบ token ด้วยการเปรียบเทียบแบบ constant-time; หากปล่อย token ว่างไว้หมายถึงปิดใช้งาน Agent API

## รายการเอนด์พอยต์

| เมธอด | พาธ | คำอธิบาย |
|------|------|------|
| GET | `/agent/v1/health` | การตรวจสุขภาพ (health check) |
| GET | `/agent/v1/version` | ข้อมูลเวอร์ชัน |
| GET | `/agent/v1/status` | สรุปสถานะ: cpu / mem / ดิสก์ / บริการออนไลน์ |
| GET | `/agent/v1/system/info` | ข้อมูลโฮสต์ (hostname / os / เคอร์เนล) |
| GET | `/agent/v1/system/processes` | รายการโปรเซส |
| GET | `/agent/v1/system/ports` | พอร์ตที่กำลัง listen |
| GET | `/agent/v1/system/disks` | ดิสก์ / จุด mount |
| GET | `/agent/v1/websites` | รายการเว็บไซต์ |
| GET | `/agent/v1/databases` | รายการฐานข้อมูล |
| GET | `/agent/v1/cron-jobs` | งานตามกำหนดเวลา |
| GET | `/agent/v1/containers` | รายการคอนเทนเนอร์ |
| POST | `/agent/v1/commands` | รันคำสั่ง (ไวท์ลิสต์) |
| POST | `/agent/v1/script/run` | รันสคริปต์ |
| POST | `/agent/v1/file/upload` | อัปโหลดไฟล์ |

## การรันคำสั่งและไวท์ลิสต์

`/commands` อนุญาตให้รันเฉพาะคำนำหน้าคำสั่งที่ได้รับอนุญาตอย่างชัดเจนเท่านั้น และทั้งหมดจะถูกบันทึกการตรวจสอบ (audit) ไวท์ลิสต์ถูกดูแลใน `agent.allowed_commands` ของ `config.yaml`:

```yaml
agent:
  allowed_commands:
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

## ตัวอย่างการเรียกใช้งาน

```bash
curl -H "Authorization: Bearer <token>" \
     https://<host>:8888/agent/v1/status
```

## คำแนะนำด้านความปลอดภัย

- ใช้การส่งข้อมูลผ่าน HTTPS
- ในสถานการณ์ที่มีความปลอดภัยสูงสามารถเพิ่ม IP ไวท์ลิสต์เพื่อจำกัดแหล่งที่มา
- ลดไวท์ลิสต์คำสั่งให้เหลือน้อยที่สุด อนุญาตเฉพาะคำสั่งที่จำเป็น
- หาก token รั่วไหล ให้สร้างใหม่ทันทีในหน้า "การตั้งค่า → API Token" ของพาเนล
