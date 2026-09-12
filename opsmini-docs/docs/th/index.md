---
title: OpsMini — พาเนลปฏิบัติการโฮสต์ Linux น้ำหนักเบา
hide:
  - navigation
  - toc
---

<div class="home-hero" markdown>

# OpsMini

**พาเนลปฏิบัติการโฮสต์ Linux น้ำหนักเบา** — มีการปฏิบัติการด้วยโมเดล AI ในตัว REST API มาตรฐานสำหรับการผสานรวมภายนอก

เทียบเคียง Baota / 1Panel ทำให้การปฏิบัติการเครื่องเดียวง่ายขึ้น ฉลาดขึ้น และผสานรวมได้มากขึ้น

<div class="home-cta" markdown>
[ติดตั้งอย่างรวดเร็ว](getting-started/quick-install.md){ .md-button .md-button--primary }
[ดูเอกสาร](getting-started/index.md){ .md-button }
[:fontawesome-brands-github: GitHub](https://github.com/unixhot/opsmini){ .md-button }
</div>

</div>

---

## ความแตกต่างหลักสองประการ

<div class="grid cards" markdown>

-   :material-robot-outline: **การปฏิบัติการด้วยโมเดล AI**

    ---

    วินิจฉัยปัญหาด้วยภาษาธรรมชาติ วิเคราะห์ล็อก รันคำสั่งปฏิบัติการ เชื่อมต่อ OpenAI / DeepSeek / Qwen / Ollama ให้ AI เป็นผู้ช่วยปฏิบัติการของคุณ

-   :material-api: **REST API มาตรฐาน**

    ---

    มี `/api/v1` (Panel API) และ `/agent/v1` (machine-to-machine API) ในตัว แพลตฟอร์มมอนิเตอร์ สคริปต์อัตโนมัติ เครื่องมือออร์เคสเตรชันใด ๆ ก็ผสานรวมและเรียกใช้ได้โดยตรง

</div>

---

## ภาพรวมฟีเจอร์

<div class="grid cards" markdown>

-   :material-view-dashboard-outline: **แดชบอร์ดและการมอนิเตอร์**

    ---

    อนุกรมเวลา CPU / หน่วยความจำ / ดิสก์ / เครือข่ายแบบเรียลไทม์ กฎการแจ้งเตือนและเหตุการณ์การแจ้งเตือนครบวงจร

-   :material-docker: **การจัดการคอนเทนเนอร์**

    ---

    ทรัพยากรสี่ประเภท Docker คอนเทนเนอร์ อิมเมจ วอลุ่ม เน็ตเวิร์กจัดการครบวงจร พร้อมร้านซอฟต์แวร์ติดตั้งแอปพลิเคชันที่พบบ่อยในคลิกเดียว

-   :material-console: **เว็บเทอร์มินัล**

    ---

    เทอร์มินัลโต้ตอบคล้าย SSH ด้วย WebSocket + pty ใช้งานเซิร์ฟเวอร์โดยตรงในเบราว์เซอร์

-   :material-shield-check-outline: **ความปลอดภัยของโฮสต์**

    ---

    การตรวจสอบ baseline การมอนิเตอร์ความสมบูรณ์ของไฟล์ (FIM) การตรวจจับภัยคุกคาม ไฟร์วอลล์ ความปลอดภัยการล็อกอิน เห็นสถานการณ์ความปลอดภัยในหน้าจอเดียว

-   :material-folder-outline: **ไฟล์และเว็บไซต์**

    ---

    การเรียกดู / อัปโหลด / แก้ไขไฟล์ การจัดการเว็บไซต์ Nginx ฐานข้อมูล ใบรับรอง SSL แบบครบวงจร

-   :material-translate: **7 ภาษา · ไบนารีเดียว**

    ---

    อินเทอร์เฟซ 7 ภาษา: จีนตัวย่อ/ตัวเต็ม อังกฤษ ญี่ปุ่น เกาหลี ไทย เยอรมัน; frontend ฝังด้วย `go:embed` static link ประมาณ 30MB ไม่มีไลบรารีรันไทม์

</div>

---

## ติดตั้งคลิกเดียว

<div class="home-section" markdown>

### เริ่มใช้ใน 5 นาที

```bash
# Linux x86_64 / aarch64 สคริปต์คลิกเดียวติดตั้งไปที่ /data/opsmini พอร์ต 8888
curl -fsSL https://opsmini.com/install.sh | sudo bash
```

เริ่มครั้งแรกสร้างบัญชีผู้ดูแล `opsmini` และรหัสผ่านสุ่มอัตโนมัติ (ดูในล็อกตอนเริ่ม) เปิด `http://<host>:8888` ก็ใช้ได้

[ดูเอกสารการติดตั้งฉบับเต็ม →](installation/index.md){ .md-button }

</div>

---

<div class="home-section" markdown>

## เริ่มทันที

deploy ด้วยคำสั่งเดียว AI เสริมการปฏิบัติการ API มาตรฐานเชื่อมการผสานรวม

[เริ่มติดตั้ง](getting-started/quick-install.md){ .md-button .md-button--primary }
[สำรวจเอกสาร](getting-started/index.md){ .md-button }

</div>
