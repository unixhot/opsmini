# การออกแบบสถาปัตยกรรม

สถาปัตยกรรมทางเทคนิคและการตัดสินใจออกแบบของ OpsMini

<div class="grid cards" markdown>

-   :material-sitemap-outline: **[สถาปัตยกรรมโดยรวม](index.md)**

    ---

    พาเนลเครื่องเดียว โมโนลิธไบนารีเดียว แยก frontend/backend + บรรจุแบบฝัง

-   :material-cube-outline: **[เทคโนโลยีที่ใช้ (Tech Stack)](tech-stack.md)**

    ---

    เหตุผลการเลือก Go · Gin · GORM · SQLite (Go ล้วน) · Vue 3 · ECharts

-   :material-folder-outline: **[โครงสร้างไดเรกทอรี](directory.md)**

    ---

    สถาปัตยกรรมแบบเลเยอร์ (Controller → Service → Repository) และการจัดระเบียบไดเรกทอรี

</div>

## การตัดสินใจสถาปัตยกรรมหลัก

- **โมโนลิธเครื่องเดียว**: ไม่ทำ microservice ส่งมอบเป็นไบนารีเดี่ยวในโปรเซสเดียว
- **SQLite**: แบบฝังตัว ไม่ต้องดูแลรักษา ปริมาณข้อมูลธุรกิจน้อยใช้งานได้เพียงพอ
- **API คู่**: `/api/v1` สำหรับ UI, `/agent/v1` สำหรับการผสานรวม แยกการยืนยันตัวตนและสิทธิ์ออกจากกัน
- **Go ล้วน ไม่ใช้ CGO**: cross-compile เป็นไบนารีแบบ static link ได้ในคลิกเดียวบนทุกแพลตฟอร์ม
