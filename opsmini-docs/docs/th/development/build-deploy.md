# การ build และ deploy

## ไลบรารีที่ต้องมีก่อน

- Go 1.25+ (ไดรเวอร์ Go ล้วน ไม่มี CGO cross-compile ได้โดยตรง)

## การรับโค้ด

```bash
git clone <ที่อยู่ repository> opsmini
cd opsmini
export GOPROXY=https://goproxy.cn,direct   # เร่งความเร็วในประเทศจีน
go mod tidy
```

## คำสั่ง build

```bash
make build          # คอมไพล์แพลตฟอร์มปัจจุบัน → dist/opsmini
make build-all      # cross-compile linux/darwin amd64/arm64
make clean          # ล้าง dist/
make version        # แสดงข้อมูลเวอร์ชัน
```

## การใส่หมายเลขเวอร์ชัน

ข้อมูล build ถูกใส่ผ่าน `-ldflags -X main.version/-X main.buildTime/-X main.gitCommit`:

```bash
make build VERSION=v1.0.0
./dist/opsmini -version
# opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

## การตั้งชื่อ artifact

`opsmini-<version>-<os>-<arch>` เช่น `opsmini-v1.0.0-linux-amd64`

## artifact ที่ส่งมอบ

| artifact | คำอธิบาย |
|------|------|
| `opsmini` ไบนารีเดียว | backend API + frontend UI + SQLite static link ประมาณ 28~30MB |
| `configs/config.yaml` | เทมเพลตการกำหนดค่า |
| `opsmini.db` | สร้างอัตโนมัติเมื่อรันครั้งแรก |

> การ deploy คือการคัดลอกไบนารี + ไฟล์กำหนดค่า ไม่มีไลบรารีรันไทม์อื่น
