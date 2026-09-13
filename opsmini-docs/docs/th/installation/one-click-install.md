# ติดตั้งด้วยสคริปต์คลิกเดียว

`install.sh` ทางการ deploy บนโฮสต์ Linux ได้ในคลิกเดียว

## เริ่มต้นอย่างรวดเร็ว

```bash
# ติดตั้งคลิกเดียว (แนะนำ สคริปต์ดาวน์โหลดไบนารีจาก Alibaba Cloud OSS ตามสถาปัตยกรรมอัตโนมัติ)
curl -fsSL https://opsmini.com/install.sh | sudo bash

# ระบุไบนารีท้องถิ่นเพื่อติดตั้ง (ติดตั้งไปที่ /data/opsmini พอร์ต 8888 เป็นค่าเริ่มต้น)
sudo ./install.sh -b ./opsmini

# กำหนดที่อยู่ดาวน์โหลดเอง (รองรับไบนารีเปล่าหรือ .tar.gz)
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# กำหนดไดเรกทอรีและพอร์ตเอง
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## พารามิเตอร์

| พารามิเตอร์ | คำอธิบาย | ค่าเริ่มต้น |
|------|------|--------|
| `-d, --dir <path>` | ไดเรกทอรีติดตั้ง | `/data/opsmini` |
| `-p, --port <port>` | พอร์ต listen ของพาเนล | `8888` |
| `-b, --binary <path>` | พาธไบนารี opsmini | ดาวน์โหลดจาก OSS เมื่อไม่ระบุ |
| `-u, --url <url>` | ดาวน์โหลดจาก URL (ไบนารีเปล่าหรือ `.tar.gz`) | ที่อยู่ทางการ OSS |
| `-n, --no-systemd` | ไม่ลงทะเบียนบริการ systemd | — |
| `-h, --help` | ความช่วยเหลือ | — |

## พฤติกรรมการติดตั้ง

1. สร้างไดเรกทอรีติดตั้ง (ค่าเริ่มต้น `/data/opsmini`)
2. ดาวน์โหลด / คัดลอกไบนารีไปที่ `<ไดเรกทอรี>/opsmini` (ดาวน์โหลดจาก Alibaba Cloud OSS ตามสถาปัตยกรรมเป็นค่าเริ่มต้น)
3. สร้าง `<ไดเรกทอรี>/config.yaml` (พอร์ต พาธ SQLite อายุ JWT ระดับล็อกและไฟล์ล็อก; คีย์ JWT และ Agent Token สร้างอัตโนมัติตอนเริ่มครั้งแรกเก็บในฐานข้อมูล)
4. สร้างรหัสผ่านผู้ดูแลแบบสุ่ม 16 ตัวอักษร เขียนไปที่ `<ไดเรกทอรี>/.init_passwd` (สิทธิ์ 600)
   - **ติดตั้งใหม่ทั้งหมด**: เริ่มครั้งแรกฉีดผ่านตัวแปรสภาพแวดล้อม `OPSMINI_INIT_PASSWORD`
   - **ติดตั้งใหม่** (ฐานข้อมูลมีอยู่แล้ว): รีเซ็ตรหัสผ่านด้วย `-reset-pass` อัตโนมัติ รหัสผ่านที่แสดงคือรหัสผ่านที่มีผล
5. ลงทะเบียนและเริ่มบริการ systemd `opsmini.service`
6. ตรวจสอบการตอบสนองของบริการ และแสดงที่อยู่เข้าถึง / ชื่อผู้ใช้ / รหัสผ่าน / พาธไฟล์ล็อก

## artifact

```
/data/opsmini/
├── opsmini              # ไบนารี
├── config.yaml          # การกำหนดค่า (600)
├── opsmini.db           # ฐานข้อมูล SQLite (สร้างหลังเริ่มครั้งแรก)
├── opsmini.log          # ไฟล์ล็อกรันไทม์
└── .init_passwd         # รหัสผ่านเริ่มต้น (600)
```

## การรับไบนารี

- **การแจกจ่ายทางการ (Alibaba Cloud OSS)**: `https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v<เวอร์ชัน>-linux-<สถาปัตยกรรม>` สคริปต์ดาวน์โหลดจากที่นี่เป็นค่าเริ่มต้น
- **การพัฒนาในเครื่อง**: `make build-all` ที่ไดเรกทอรีรากของโปรเจกต์ให้ artifact `dist/opsmini-<ver>-linux-amd64` / `-arm64` สคริปต์จะค้นหาอัตโนมัติตามสถาปัตยกรรม

## ความเข้ากันได้

- รองรับเฉพาะ `x86_64` / `aarch64`
- สภาพแวดล้อมที่ไม่มี systemd ใช้ `-n` เพื่อข้ามการลงทะเบียนบริการ ใช้การเริ่มด้วยตนเองแทน

## การกู้คืนบัญชี {: #account-recovery }

เมื่อลืมรหัสผ่านหรือทำรหัสยืนยัน MFA หาย ให้รันบนเซิร์ฟเวอร์ (หยุดบริการก่อน แล้วเริ่มใหม่หลังเสร็จ):

```bash
# รีเซ็ตรหัสผ่าน
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-pass opsmini
systemctl start opsmini

# ล้างการผูก MFA
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-mfa opsmini
systemctl start opsmini
```

> หมายเหตุ: `-reset-mfa` / `-reset-pass` เป็นคำสั่งย่อยกู้คืนบัญชี ดำเนินการกับฐานข้อมูล SQLite โดยตรงแล้วออก ไม่เริ่มบริการเว็บ ต้องแน่ใจว่าบริการอยู่ในสถานะหยุดระหว่างการดำเนินการ
