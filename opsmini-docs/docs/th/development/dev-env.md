# สภาพแวดล้อมการพัฒนาในเครื่อง

## ติดตั้ง Go 1.25+

```bash
# macOS (Apple Silicon)
curl -sL -o /tmp/go.tar.gz https://go.dev/dl/go1.25.0.darwin-arm64.tar.gz
mkdir -p ~/go-sdk && tar -C ~/go-sdk -xzf /tmp/go.tar.gz
export PATH=~/go-sdk/go/bin:$PATH

# Linux x64
curl -sL -o /tmp/go.tar.gz https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf /tmp/go.tar.gz
export PATH=/usr/local/go/bin:$PATH
```

## การรันพาเนล

```bash
go run ./cmd/agent -config configs/config.yaml
# หรือ build แล้วรัน
make build && ./dist/opsmini -config configs/config.yaml
```

เข้าถึง `http://localhost:8888` บัญชีเริ่มต้น `opsmini` รหัสผ่านดูในล็อกตอนเริ่ม

## หมายเหตุ frontend

- frontend เป็นไฟล์เดียว `web/index.html` (Vue 3 SPA แบบ inline) + ไลบรารี static `web/static/`
- ฝังเข้าไบนารีผ่าน `go:embed` ใน `web/embed.go`
- หลังแก้ไข frontend ให้ `make build` ใหม่เพื่อให้มีผล

## ปัญหาที่พบบ่อย

- `go mod tidy` ค้าง → กำหนดค่า `GOPROXY=https://goproxy.cn,direct`
- build แจ้งข้อขัดแย้งไดเรกทอรี `vendor` → ไดเรกทอรีไลบรารี frontend เปลี่ยนชื่อเป็น `static/` แล้ว อย่าสร้าง `vendor/` อีก
