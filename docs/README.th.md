<div align="center">

# OpsMini

**แผงจัดการเซิร์ฟเวอร์ที่ขับเคลื่อนด้วย AI**

[English](README.en.md) · [简体中文](../README.md) · [繁體中文](README.zh-TW.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [ไทย](README.th.md) · [Deutsch](README.de.md)

</div>

---

## ไทย

OpsMini เป็นแผงจัดการเซิร์ฟเวอร์ที่ขับเคลื่อนด้วย AI (เทียบเท่า BaoTa / 1Panel) โดยมีจุดเด่นคือ AI ปฏิบัติการในตัวและ REST API มาตรฐานสำหรับการเชื่อมต่อภายนอก

### คุณสมบัติ

- **AI ปฏิบัติการ** — วินิจฉัยด้วยภาษาธรรมชาติ วิเคราะห์ล็อก และรันคำสั่ง
- **REST API มาตรฐาน** — `/api/v1` (UI บราวเซอร์) และ `/agent/v1` (ระหว่างเครื่อง)
- **จัดการ Docker** — คอนเทนเนอร์ อิมเมจ วอลุ่ม และเครือข่าย
- **เทอร์มินัลเว็บ** — เชลล์แบบโต้ตอบคล้าย SSH ผ่าน WebSocket + pty
- **i18n 7 ภาษา** — จีนตัวย่อ/ตัวเต็ม อังกฤษ ญี่ปุ่น เกาหลี ไทย และเยอรมัน
- **ไบนารีเดี่ยว** — ฝังฟรอนต์เอนด์ด้วย `go:embed` ไม่มี dependency ขณะรันไทม์

### สแต็กเทคโนโลยี

Go · Gin · GORM · SQLite (Go แท้) · Vue 3 · ECharts

### เริ่มต้นใช้งาน

```bash
make build          # บิลด์สำหรับแพลตฟอร์มปัจจุบัน
make build-all      # ครอสคอมไพล์สำหรับ Linux amd64/arm64

./dist/opsmini -config configs/config.yaml
# เปิด http://localhost:8888 (บัญชีเริ่มต้น: opsmini ดูรหัสผ่านในล็อกเริ่มต้น)
```

### เอกสาร

- [คู่มือนักพัฒนา](docs/developer-guide.md)
- [สถาปัตยกรรมแบ็กเอนด์](docs/backend-architecture.md)
- [การบิลด์และการปรับใช้](docs/build-and-deploy.md)
- [การออกแบบความปลอดภัยโฮสต์](docs/security-audit.md)

### ลิขสิทธิ์

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
