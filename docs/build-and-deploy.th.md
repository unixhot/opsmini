<div align="center">

# OpsMini การบิลด์และการปรับใช้

**คู่มือการบิลด์และการปรับใช้**

[English](build-and-deploy.md) · [简体中文](build-and-deploy.zh-CN.md) · [繁體中文](build-and-deploy.zh-TW.md) · [日本語](build-and-deploy.ja.md) · [한국어](build-and-deploy.ko.md) · [ไทย](build-and-deploy.th.md) · [Deutsch](build-and-deploy.de.md)

</div>

---

> เวอร์ชัน: v1.0  
> ขอบเขต: กระบวนการทั้งหมดตั้งแต่การบิลด์จากซอร์สโค้ดจนถึงการปรับใช้ในโปรดักชัน

---

## 1. ข้อกำหนดเบื้องต้น

| ข้อกำหนด | เวอร์ชันที่ต้องการ | คำอธิบาย |
|------|----------|------|
| Go | **1.25+** | ขับเคลื่อนด้วย Go ล้วน ไม่มี CGO ครอสคอมไพล์ได้โดยตรงบนทุกแพลตฟอร์ม |
| หน่วยความจำ | ≥ 512MB | เบาทั้งตอนบิลด์และตอนรัน |
| โฮสต์เป้าหมาย | Linux / macOS | งานเซิร์ฟเวอร์ใช้ Linux x64 / arm64 ส่วน macOS ใช้สำหรับพัฒนาและดีบักเท่านั้น |

> **เหตุใดจึงไม่ต้องใช้ CGO toolchain**: ไดรเวอร์ SQLite ใช้ `github.com/glebarez/sqlite` (เขียนด้วย Go ล้วน)
> จึงสามารถครอสคอมไพล์ไบนารีแบบ **static linked** ได้บนทุกแพลตฟอร์มด้วย `CGO_ENABLED=0` โดยไม่ต้องตั้งค่า C cross-compiler แยกสำหรับแต่ละสถาปัตยกรรมเป้าหมาย

---

## 2. ดึงโค้ดและ dependencies

```bash
git clone <仓库地址> opsmini
cd opsmini

# 国内网络需配置 goproxy 镜像（proxy.golang.org 可能被墙）
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

---

## 3. การบิลด์

### 3.1 คอมไพล์สำหรับแพลตฟอร์มปัจจุบัน

```bash
make build
# 产物：dist/opsmini
```

### 3.2 ครอสคอมไพล์ทุกแพลตฟอร์มเป้าหมาย

```bash
make build-all
# 产物：
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64
```

| แพลตฟอร์ม | สถานการณ์ใช้งาน |
|------|----------|
| `linux/amd64` | เซิร์ฟเวอร์ x86_64 ทั่วไป (โปรดักชัน) |
| `linux/arm64` | เซิร์ฟเวอร์ ARM (Graviton / Raspberry Pi / Kunpeng / Phytium, โปรดักชัน) |
| `darwin/amd64` | เครื่องพัฒนา Intel Mac (พัฒนาและดีบัก) |
| `darwin/arm64` | เครื่องพัฒนา Apple Silicon (พัฒนาและดีบัก) |

> OpsMini เป็นแพเนลสำหรับ **โฮสต์ Linux** โดยเผยแพร่เฉพาะแพ็กเกจติดตั้ง Linux เท่านั้น เป้าหมาย macOS ใช้สำหรับพัฒนาและดีบักในเครื่องเท่านั้น ไม่ได้ใช้เป็นผลลัพธ์ส่งมอบ

### 3.3 การใส่หมายเลขเวอร์ชัน

```bash
# 默认取 git tag / commit，也可显式指定
make build VERSION=v1.0.0

# 查看二进制版本
./dist/opsmini -version
# 输出：opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

> ขั้นตอนการออกรุ่นอย่างเป็นทางการ: `git tag v1.0.0 && make build-all` ระบบจะดึง `VERSION` จากชื่อ tag โดยอัตโนมัติ

### 3.4 เป้าหมาย Makefile อื่นๆ

```bash
make clean     # 清理 dist/
make version   # 打印当前版本信息
```

### 3.5 ตรวจสอบสถาปัตยกรรมของผลลัพธ์

```bash
file dist/*
# 预期：ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64，均为静态链接
```

---

## 4. การกำหนดค่า

เส้นทางเริ่มต้นของไฟล์คอนฟิกคือ `configs/config.yaml` (ระบุได้ด้วย `-config`) ตัวอย่างฉบับสมบูรณ์:

```yaml
server:
  host: "0.0.0.0"              # 监听地址
  port: 8888                    # 监听端口
  secret_entry: ""              # 安全入口前缀，如 /opsmini_panel（留空不启用）

database:
  path: "opsmini.db"            # SQLite 数据文件路径

jwt:
  secret: "change-me"           # JWT 签名密钥，生产环境务必修改为随机串
  access_ttl_seconds: 900       # access token 有效期（15 分钟）
  refresh_ttl_seconds: 604800   # refresh token 有效期（7 天）

ai:
  enabled: true                 # 是否启用 AI 助手
  provider: "openai"            # openai / deepseek / qwen / ollama
  model: "gpt-4o"               # 模型名
  base_url: "https://api.openai.com/v1"
  api_key: ""                   # 建议用环境变量 OPSMINI_AI_KEY 覆盖

agent:
  token: ""                     # 对外 REST API 认证令牌，空则禁用 /agent/v1
  allowed_commands:             # 命令白名单前缀（仅允许以此开头的命令）
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

### ตัวแปรสภาพแวดล้อม

| ตัวแปร | คำอธิบาย |
|------|------|
| `OPSMINI_AI_KEY` | AI API Key มี**ลำดับความสำคัญสูงกว่า** `ai.api_key` (หลีกเลี่ยงการบันทึกคีย์ลงดิสก์) |

---

## 5. การรัน

```bash
# 直接运行
./opsmini -config configs/config.yaml

# 打印版本并退出
./opsmini -version
```

- เมื่อเริ่มต้นครั้งแรกจะสร้างตารางและเขียนบัญชีผู้ดูแลระบบเริ่มต้นโดยอัตโนมัติ
- เข้าถึง `http://<host>:8888` ก็จะพบแพเนล (ฟรอนต์เอนด์ฝังอยู่ในไบนารีแล้ว ไม่ต้องดีพลอยแยก)

### บัญชีเริ่มต้น

| รายการ | ค่า |
|----|----|
| ชื่อผู้ใช้ | `opsmini` (คงที่) |
| รหัสผ่าน | **สุ่มสร้างตอนเริ่มต้นครั้งแรก** ดูได้จากล็อกการเริ่มต้น |

หลังเริ่มต้น ล็อกจะแสดงข้อความทำนองนี้:

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

> ⚠️ โปรดบันทึกรหัสผ่านนี้ทันทีและเปลี่ยนหลังเข้าสู่ระบบ รหัสผ่านจะถูกสร้างเพียงครั้งเดียวตอนติดตั้งครั้งแรก หลังจากนั้นต้องเปลี่ยนผ่าน "การจัดการผู้ใช้" ในแพเนล

---

## 6. การปรับใช้ในโปรดักชัน (Linux + systemd)

### 6.1 การจัดวางไดเรกทอรี

```
/opt/opsmini/
├── opsmini              # 二进制
└── config.yaml          # 配置文件
/var/lib/opsmini/
└── opsmini.db           # 数据（自动生成，随 data 目录权限而定）
```

### 6.2 ขั้นตอนการติดตั้ง

```bash
# 1. 放置二进制与配置
sudo mkdir -p /opt/opsmini /var/lib/opsmini
sudo cp dist/opsmini-*-linux-amd64 /opt/opsmini/opsmini
sudo cp configs/config.yaml /opt/opsmini/config.yaml

# 2. 修改配置：JWT secret、数据库绝对路径
sudo sed -i 's|path: "opsmini.db"|path: "/var/lib/opsmini/opsmini.db"|' /opt/opsmini/config.yaml
sudo sed -i 's|secret: "change-me"|secret: "<随机长串>"|' /opt/opsmini/config.yaml

# 3. 赋权
sudo chmod +x /opt/opsmini/opsmini
```

### 6.3 บริการ systemd

สร้าง `/etc/systemd/system/opsmini.service`:

```ini
[Unit]
Description=OpsMini Server Panel
After=network.target

[Service]
Type=simple
ExecStart=/opt/opsmini/opsmini -config /opt/opsmini/config.yaml
Restart=always
RestartSec=3
# 安全加固（可选）
User=opsmini
Group=opsmini
NoNewPrivileges=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
```

เปิดใช้งานและเริ่มต้น:

```bash
sudo useradd -r -s /usr/sbin/nologin opsmini || true
sudo chown -R opsmini:opsmini /var/lib/opsmini /opt/opsmini
sudo systemctl daemon-reload
sudo systemctl enable --now opsmini
sudo systemctl status opsmini
```

---

## 7. Reverse proxy (ไม่บังคับ)

### 7.1 Nginx

```nginx
server {
    listen 80;
    server_name panel.example.com;

    location / {
        proxy_pass http://127.0.0.1:8888;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Web 终端（WebSocket）需要升级支持
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}
```

### 7.2 Caddy (HTTPS อัตโนมัติ)

```caddyfile
panel.example.com {
    reverse_proxy 127.0.0.1:8888
}
```

### 7.3 Secret entry

หากกำหนดค่า `secret_entry: /opsmini_panel` เส้นทางของแพเนลจะกลายเป็น `http://<host>:8888/opsmini_panel` ซึ่งใช้ร่วมกับ reverse proxy เพื่อซ่อนทางเข้าจริงได้

---

## 8. การอัปเกรด

```bash
# 1. 备份数据
sudo systemctl stop opsmini
cp /var/lib/opsmini/opsmini.db /var/lib/opsmini/opsmini.db.bak.$(date +%s)

# 2. 替换二进制
sudo cp dist/opsmini-<new>-linux-amd64 /opt/opsmini/opsmini

# 3. 重启
sudo systemctl start opsmini
```

> SQLite ถูกไมเกรตอัตโนมัติโดย GORM การอัปเกรดข้ามเวอร์ชันโดยทั่วไปไม่ต้องแก้ตารางด้วยตนเอง

---

## 9. คำถามที่พบบ่อย

| ปัญหา | สาเหตุ | วิธีแก้ |
|------|------|------|
| `go mod tidy` ค้าง/ขึ้น Bad Gateway | `proxy.golang.org` ถูกบล็อก | `export GOPROXY=https://goproxy.cn,direct` |
| แจ้งว่าเวอร์ชัน Go ต่ำเกินไป | dependencies ต้องการ Go 1.25+ | ดาวน์โหลดไบนารีที่คอมไพล์แล้วจากทางการ ดูด้านล่าง |
| บิลด์แล้วเจอความขัดแย้งไดเรกทอรี `vendor` | `vendor/` ในโปรเจกต์ขัดกับข้อกำหนด vendor ของ Go module | ไดเรกทอรี dependencies ของฟรอนต์เอนด์ถูกเปลี่ยนชื่อเป็น `static/` แล้ว อย่าสร้าง `vendor/` ใหม่ |
| เข้าถึง `/` แล้วได้ 301 `./` | `c.FileFromFS` ทำ directory redirect ของ embed.FS | แก้แล้วโดยใช้ ReadFile + c.Data (อย่าย้อนกลับ) |

### ติดตั้ง Go 1.25+ (ไบนารีทางการ เร็วที่สุด)

```bash
# macOS（Apple Silicon）
curl -sL -o /tmp/go.tar.gz https://go.dev/dl/go1.25.0.darwin-arm64.tar.gz
mkdir -p ~/go-sdk && tar -C ~/go-sdk -xzf /tmp/go.tar.gz
export PATH=~/go-sdk/go/bin:$PATH

# Linux x64
curl -sL -o /tmp/go.tar.gz https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf /tmp/go.tar.gz
export PATH=/usr/local/go/bin:$PATH
```

---

## 10. รายการผลลัพธ์ส่งมอบ

| ผลลัพธ์ | คำอธิบาย |
|------|------|
| ไบนารีเดี่ยว `opsmini` | Backend API + Frontend UI + SQLite ขนาดประมาณ 28~30MB แบบ static linked |
| `configs/config.yaml` | เทมเพลตคอนฟิก |
| `opsmini.db` | ไฟล์ข้อมูลที่สร้างอัตโนมัติเมื่อรันครั้งแรก |

> การดีพลอยคือการคัดลอกไบนารี + ไฟล์คอนฟิกเท่านั้น ไม่มี runtime dependency อื่นใด
