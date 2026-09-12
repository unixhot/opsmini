# REST API

OpsMini เปิดเผย REST API มาตรฐานสองชุดสำหรับใช้งานโดย UI ของเบราว์เซอร์และการผสานรวมกับระบบภายนอก

## ภาพรวม API

| API | คำนำหน้า (Prefix) | ผู้ใช้งาน | การยืนยันตัวตน |
|-----|------|--------|------|
| Panel API | `/api/v1` | เบราว์เซอร์ (คน) | เซสชันผู้ใช้ JWT + RBAC |
| Agent API | `/agent/v1` | ระบบภายนอก (เครื่อง) | Agent Token + ไวท์ลิสต์คำสั่ง |
| Prometheus metrics | `/metrics` | ระบบมอนิเตอร์ | Basic authentication (ทางเลือก) |

รูปแบบการตอบกลับมาตรฐาน: `{ "code": 0, "message": "ok", "data": ... }` โดย `code` ที่ไม่ใช่ 0 หมายถึงข้อผิดพลาดทางธุรกิจ

<div class="grid cards" markdown>

-   :material-web: **[Panel API (/api/v1)](browser-api.md)**

    ---

    ความสามารถ UI ทั้งหมด ได้แก่ การยืนยันตัวตน ผู้ใช้ การมอนิเตอร์ เว็บไซต์ คอนเทนเนอร์ ไฟล์ AI เป็นต้น

-   :material-api: **[Agent API (/agent/v1)](agent-api.md)**

    ---

    อินเทอร์เฟซการจัดการทรัพยากรและการรันคำสั่งแบบเครื่องต่อเครื่อง

-   :material-chart-line: **[Prometheus /metrics](prometheus.md)**

    ---

    เมตริกที่เข้ากันได้กับ node_exporter Prometheus สามารถ scrape ได้โดยตรง

</div>
