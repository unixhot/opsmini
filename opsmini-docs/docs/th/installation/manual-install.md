# ติดตั้งด้วยตนเอง (build จากซอร์สโค้ด)

เมื่อไม่ต้องการใช้สคริปต์คลิกเดียว สามารถ build จากซอร์สโค้ดและ deploy ด้วยตนเองได้

## ไลบรารีที่ต้องมีก่อน

| ไลบรารี | ข้อกำหนดเวอร์ชัน | คำอธิบาย |
|------|----------|------|
| Go | **1.25+** | ไดรเวอร์ Go ล้วน ไม่มี CGO cross-compile ได้โดยตรงบนทุกแพลตฟอร์ม |
| หน่วยความจำ | ≥ 512MB | build และรันเบาทั้งคู่ |

> **ทำไมไม่ต้องใช้ toolchain CGO**: ไดรเวอร์ SQLite ใช้ `github.com/glebarez/sqlite` (Go ล้วน)
> `CGO_ENABLED=0` ก็สามารถ cross-compile ไบนารีแบบ **static link** บนทุกแพลตฟอร์มได้

## รับโค้ดและไลบรารี

```bash
git clone <ที่อยู่ repository> opsmini
cd opsmini

# เครือข่ายในประเทศจีนต้องกำหนดค่า goproxy mirror
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

## การ build

```bash
# คอมไพล์แพลตฟอร์มปัจจุบัน → dist/opsmini
make build

# cross-compile ทุกแพลตฟอร์มเป้าหมาย
make build-all
# artifact:
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64

# ระบุหมายเลขเวอร์ชันอย่างชัดเจน
make build VERSION=v1.0.0

# ดูเวอร์ชันไบนารี
./dist/opsmini -version
```

> ขั้นตอนการเผยแพร่ทางการ: `git tag v1.0.0 && make build-all` โดย `VERSION` จะนำชื่อ tag มาใช้โดยอัตโนมัติ

## ตรวจสอบสถาปัตยกรรมของ artifact

```bash
file dist/*
# คาดว่า: ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64 ทั้งหมดเป็น static link
```

## การ deploy ด้วยตนเอง

### การวางแผนไดเรกทอรี

```
/data/opsmini/
├── opsmini              # ไบนารี
├── config.yaml          # ไฟล์กำหนดค่า
└── opsmini.db           # ข้อมูล (สร้างอัตโนมัติเมื่อเริ่มครั้งแรก)
```

### วางไบนารีและการกำหนดค่า

```bash
sudo mkdir -p /data/opsmini
sudo cp dist/opsmini-*-linux-amd64 /data/opsmini/opsmini
sudo cp configs/config.yaml /data/opsmini/config.yaml

# แก้ไขการกำหนดค่า: พาธฐานข้อมูลแบบ absolute (คีย์เซ็น JWT ไม่ต้องกำหนดค่า สร้างอัตโนมัติตอนเริ่มครั้งแรก)
sudo sed -i 's|path: "opsmini.db"|path: "/data/opsmini/opsmini.db"|' /data/opsmini/config.yaml

sudo chmod +x /data/opsmini/opsmini
```

### การรัน

```bash
/data/opsmini/opsmini -config /data/opsmini/config.yaml
```

ตอนเริ่มครั้งแรกจะสร้างตารางอัตโนมัติและเขียนบัญชีผู้ดูแลเริ่มต้น ล็อกจะแสดงบัญชีและรหัสผ่าน เข้าถึง `http://<host>:8888` ก็จะเห็นพาเนล

## ขั้นตอนถัดไป

- [จัดการด้วย systemd](systemd.md)
- [รายละเอียดการกำหนดค่า](../configuration/config-file.md)
