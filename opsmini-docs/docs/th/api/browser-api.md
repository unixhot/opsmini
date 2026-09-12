# Panel API (/api/v1)

Panel API มีไว้สำหรับ UI ของเบราว์เซอร์ ใช้การยืนยันตัวตนด้วยเซสชันผู้ใช้ JWT + RBAC

## การยืนยันตัวตน

- อินเทอร์เฟซล็อกอิน `POST /auth/login` ส่งคืน token `access` + `refresh`
- คำขอถัดไปจะแนบ `Authorization: Bearer <access_token>`
- เมื่อ access หมดอายุ ให้ใช้ refresh เพื่อขอ token ใหม่
- การล็อกอินมีการจำกัดอัตรา (rate limit) (ป้องกัน brute force); เมื่อเปิดใช้ 2FA แล้ว การล็อกอินต้องเรียก `POST /auth/mfa/verify` เพื่อตรวจสอบรหัสแบบไดนามิกก่อน

## การตอบกลับมาตรฐาน

```json
{ "code": 0, "message": "ok", "data": { } }
```

`code` ที่ไม่ใช่ 0 หมายถึงข้อผิดพลาดทางธุรกิจ

## รายการเอนด์พอยต์

### การยืนยันตัวตนและผู้ใช้

| เมธอด | พาธ | คำอธิบาย |
|------|------|------|
| POST | `/auth/login` | ล็อกอิน (จำกัดอัตรา) |
| POST | `/auth/refresh` | รีเฟรช token |
| POST | `/auth/logout` | ออกจากระบบ |
| POST | `/auth/mfa/verify` | ตรวจสอบรหัส MFA แบบไดนามิกตอนล็อกอิน |
| GET | `/auth/mfa/status` | สอบถามสถานะการผูก MFA ของผู้ใช้ปัจจุบัน |
| POST | `/auth/mfa/setup` | สร้าง QR code / คีย์สำหรับผูก MFA |
| POST | `/auth/mfa/enable` | ตรวจสอบและเปิดใช้ MFA |
| POST | `/auth/mfa/disable` | ยกเลิกการผูก MFA |
| GET | `/profile` | ข้อมูลส่วนตัว (ชื่อเล่น / รูปประจำตัว / อีเมล) |
| PUT | `/profile` | แก้ไขข้อมูลส่วนตัว |
| POST | `/profile/password` | เปลี่ยนรหัสผ่าน |
| GET/POST/PUT/DELETE | `/users` | CRUD ผู้ใช้ |
| GET/POST/PUT/DELETE | `/roles` | CRUD บทบาท |
| GET | `/roles/groups` | กลุ่มสิทธิ์ |
| GET | `/permissions` | สิทธิ์ของผู้ใช้ปัจจุบัน |

### การตั้งค่าพาเนล

| เมธอด | พาธ | คำอธิบาย |
|------|------|------|
| GET | `/settings` | อ่านการตั้งค่าที่ไม่ละเอียดอ่อนทั้งหมด (`settings.view`) |
| PUT | `/settings` | อัปเดตการตั้งค่าแบบกลุ่ม (`settings.edit`) |

> รายการละเอียดอ่อน (`jwt_secret`, `metrics_pass`) จะไม่ถูกส่งคืน; รายการที่ป้องกันการเขียน (`jwt_secret`) ไม่สามารถเขียนทับได้ ดูรายละเอียดที่ [การตั้งค่าพาเนล](../configuration/panel-settings.md)

### การมอนิเตอร์และการแจ้งเตือน

| เมธอด | พาธ | คำอธิบาย |
|------|------|------|
| GET | `/dashboard/overview` | ภาพรวมแดชบอร์ด |
| GET | `/dashboard/metrics` | เมตริกแดชบอร์ด |
| GET | `/system/monitor` | สรุปการมอนิเตอร์ |
| CRUD | `/alert-rules` | กฎการแจ้งเตือน |
| GET | `/alert-events` | เหตุการณ์การแจ้งเตือน |

### การแจ้งเตือน

| เมธอด | พาธ | คำอธิบาย |
|------|------|------|
| GET | `/notifications` | รายการการแจ้งเตือน |
| GET | `/notifications/unread-count` | จำนวนที่ยังไม่อ่าน |
| PUT | `/notifications/read-all` | ทำเครื่องหมายอ่านทั้งหมด |
| PUT | `/notifications/:id/read` | ทำเครื่องหมายว่าอ่านแล้ว |
| DELETE | `/notifications/:id` / `/notifications` | ลบรายการเดียว / ล้างทั้งหมด |

### การจัดการทรัพยากร

| เมธอด | พาธ | คำอธิบาย |
|------|------|------|
| CRUD | `/websites` | เว็บไซต์ |
| CRUD | `/databases` | ฐานข้อมูล |
| GET/POST | `/apps` `/app-categories` | ร้านซอฟต์แวร์และหมวดหมู่ |
| GET/POST | `/containers` `/images` `/volumes` `/networks` | ทรัพยากรคอนเทนเนอร์สี่ประเภท |
| GET/POST | `/files` | ไฟล์ |
| CRUD | `/cron-jobs` | งานตามกำหนดเวลา |
| GET | `/logs` `/logs/tail` | ล็อก |

### ระบบและความปลอดภัย

| เมธอด | พาธ | คำอธิบาย |
|------|------|------|
| GET | `/system/info` `/processes` `/ports` `/disks` `/network` `/users` `/groups` `/firewall` | ทรัพยากรระบบ |
| GET/POST | `/security/...` | ความปลอดภัยของโฮสต์ (baseline / FIM / ภัยคุกคาม / ไฟร์วอลล์ / ความปลอดภัยการล็อกอิน) |
| GET | `/audit-logs` `/access-logs` | ล็อกการตรวจสอบ / การเข้าถึง |

### AI และการผสานรวม

| เมธอด | พาธ | คำอธิบาย |
|------|------|------|
| POST | `/ai/chat` | สนทนา AI (function call) |
| POST | `/ai/chat/stream` | สนทนา AI (SSE แบบสตรีม) |
| GET/POST/PUT/DELETE | `/mcp` | การกำหนดค่า MCP |
| GET/POST/DELETE | `/skills` ฯลฯ | ทักษะ AI (SkillHub ค้นหา / ติดตั้ง / อัปโหลด) |

### เทอร์มินัล

| เมธอด | พาธ | คำอธิบาย |
|------|------|------|
| GET | `/terminal` | เทอร์มินัล WebSocket (ยืนยันตัวตนด้วย query token) |
| GET | `/containers/:id/exec` | เทอร์มินัล WebSocket ของคอนเทนเนอร์ |

## หมายเหตุ

- การเขียนทั้งหมดถูกควบคุมโดยจุดสิทธิ์ RBAC (เช่น `website.create`, `container.edit`, `settings.edit`)
- การดำเนินการสำคัญจะเขียนล็อกการตรวจสอบ คำขอในสถานะล็อกอินจะเขียนล็อกการเข้าถึง
