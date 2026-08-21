<div align="center">

# OpsMini Developer Guide

**คู่มือนักพัฒนา**

[English](developer-guide.md) · [简体中文](developer-guide.zh-CN.md) · [繁體中文](developer-guide.zh-TW.md) · [日本語](developer-guide.ja.md) · [한국어](developer-guide.ko.md) · [ไทย](developer-guide.th.md) · [Deutsch](developer-guide.de.md)

</div>

---

> เวอร์ชัน: v1.0.0 · ภาษา: ไทย · [English](developer-guide.md)

คู่มือนี้จะอธิบายโค้ดเบสของ OpsMini อย่างละเอียด: สถาปัตยกรรม โครงสร้างไดเรกทอรี ทุกโมดูล การออกแบบ API โมเดลข้อมูล RBAC และวิธีการขยายระบบ

---

## 1. ภาพรวม

OpsMini เป็น**แผงจัดการเซิร์ฟเวอร์โฮสต์เดี่ยวแบบเบา** (คล้าย BaoTa / 1Panel) แตกต่างด้วยสองสิ่ง:

1. **การปฏิบัติการด้วย AI** — อะแดปเตอร์ LLM ช่วยให้คุณจัดการระบบด้วยภาษาธรรมชาติ (วินิจฉัย วิเคราะห์ล็อก รันคำสั่ง)
2. **สัญญา REST ที่สะอาด** — แผงเปิดเผย `/api/v1` (สำหรับ UI เบราว์เซอร์) และ `/agent/v1` (เครื่องต่อเครื่อง) เพื่อให้พร็อกซีส่วนกลางดึงเมตริกและจัดการโฮสต์จำนวนมากได้

จัดส่งเป็น**ไบนารีสแตติกเดี่ยว**: ฟรอนต์เอนด์ Vue3 ถูกฝังผ่าน `go:embed` ส่วน SQLite ใช้ไดรเวอร์ Go ล้วน (ไม่มี CGO) ดังนั้นการคอมไพล์ข้ามแพลตฟอร์มด้วย `CGO_ENABLED=0` จึงทำงานได้สำหรับ linux/amd64, linux/arm64, darwin และ windows

### เทคโนโลยีสแต็ก

| เลเยอร์ | เทคโนโลยี |
|---------|-----------|
| แบ็กเอนด์ | Go, Gin, GORM, `glebarez/sqlite` (Go ล้วน), gopsutil/v4, JWT (golang-jwt/v5), bcrypt, robfig/cron/v3 |
| ฟรอนต์เอนด์ | Vue 3 (SPA ไฟล์เดียว), ECharts, xterm.js (เว็บเทอร์มินัล) |
| การจัดเก็บ | SQLite |
| การจัดส่ง | ไบนารีเดี่ยว (ฟรอนต์เอนด์ `go:embed`) |

---

## 2. สถาปัตยกรรม

แบ็กเอนด์ใช้สถาปัตยกรรมแบบเลเยอร์ที่สะอาด:

```
คำขอ HTTP
   │
   ▼
router (gin.Engine, การลงทะเบียนเส้นทาง + การต่อมิดเดิลแวร์)
   │
   ▼
middleware (การยืนยันตัวตน → สิทธิ์ RBAC → การตรวจสอบ → บันทึกการเข้าถึง)
   │
   ▼
api/v1 handler (แยก/ตรวจสอบคำขอ เรียกใช้ service เขียนการตอบกลับ)
   │
   ▼
service (ตรรกะธุรกิจ การประสาน ระบบภายนอก)
   │
   ▼
repository (การเข้าถึงข้อมูล GORM)
   │
   ▼
model (struct ที่แมปกับตาราง SQLite)
```

กฎ:

- **handler** ทำเพียงการแยก/ตรวจสอบคำขอและการจัดรูปแบบการตอบกลับ — ไม่มีตรรกะธุรกิจ
- **service** บรรจุตรรกะธุรกิจและประสาน repository + ทรัพยากรระบบ (gopsutil, Docker SDK, crontab, pty)
- **repository** ห่อหุ้ม GORM และเป็นเลเยอร์เดียวที่เข้าถึงฐานข้อมูล
- **model** คือ GORM struct ที่มีการแมปตารางและ JSON tag

### โฟลว์คำขอ (API แผงที่ยืนยันตัวตนแล้ว)

```
เบราว์เซอร์ ──► /api/v1/xxx
             │  authMw      (แยก JWT, ฉีด id/role ของผู้ใช้)
             │  permMw      (ตรวจสอบสิทธิ์ RBAC, เลือกได้ต่อเส้นทาง)
             │  auditMw     (บันทึกบันทึกการตรวจสอบ)
             │  accessLogMw (บันทึกบันทึกการเข้าถึง)
             ▼
          handler → service → repository → SQLite
```

Agent API (`/agent/v1/*`) ใช้การยืนยันตัวตนแบบ **Agent Token แยกต่างหาก** (ไม่ใช่ JWT ของผู้ใช้) ซึ่งป้องกันโดย `middleware.AgentAuth`

---

## 3. โครงสร้างไดเรกทอรี

```
opsmini/
├── cmd/
│   └── agent/main.go        # จุดเริ่มต้น: แยกแฟล็ก, โหลดคอนฟิก, เริ่มต้น DB, เริ่ม HTTP
├── configs/
│   └── config.yaml          # เทมเพลตคอนฟิกเริ่มต้น
├── internal/
│   ├── config/              # การโหลดคอนฟิก (Config struct + YAML + ค่าเริ่มต้น)
│   ├── router/              # เราเตอร์ gin, การลงทะเบียนเส้นทาง, การต่อมิดเดิลแวร์
│   ├── middleware/          # auth, perm (RBAC), audit, accesslog, cors, ratelimit, metrics_auth, agent
│   ├── api/v1/              # HTTP handler (เอ็นด์พอยต์แผง + agent)
│   ├── service/             # ตรรกะธุรกิจ (หนึ่งไฟล์ต่อโมดูล)
│   ├── repository/          # การเข้าถึงข้อมูล GORM (หนึ่งไฟล์ต่อโมเดล)
│   ├── model/               # GORM model + กลุ่มสิทธิ์ RBAC
│   └── pkg/
│       ├── jwt/             # การลงนาม/ตรวจสอบ JWT (access + refresh)
│       ├── response/        # ซองการตอบกลับ API แบบรวม
│       └── store/           # การเริ่มต้น SQLite, การย้ายสคีมาอัตโนมัติ, ข้อมูล seed
├── web/
│   ├── index.html           # Vue3 SPA ไฟล์เดียว (i18n 7 ภาษา, ธีม)
│   ├── embed.go             # go:embed ฝังฟรอนต์เอนด์ลงในไบนารี
│   └── static/              # ดีเพนเดนซีฟรอนต์เอนด์แบบออฟไลน์
├── install/                 # สคริปต์ติดตั้ง
├── docs/                    # เอกสารโปรเจกต์
└── Makefile                 # เป้าหมายบิลด์ / คอมไพล์ข้าม / เวอร์ชัน
```

---

## 4. อ้างอิงโมดูล

### 4.1 `cmd/agent/main.go`

จุดเริ่มต้น หน้าที่:

- แยกแฟล็ก (`-config`, `-version`);
- โหลดคอนฟิกผ่าน `internal/config`;
- เปิด SQLite ผ่าน `internal/pkg/store`;
- seed บทบาทในตัวและบัญชี admin เริ่มต้น;
- สร้าง service + handler และส่งให้ `internal/router`;
- เริ่มเซิร์ฟเวอร์ HTTP (และเอ็นด์พอยต์ metrics เสริม)

### 4.2 `internal/config`

`config.go` กำหนด `Config` struct และโหลด YAML จากเส้นทาง `-config` เมื่อไฟล์หายไปจะคืนค่าเริ่มต้นในตัว ส่วน: `server`, `database`, `jwt`, `ai`, `agent`

### 4.3 `internal/router`

`router.go` เป็นที่เดียวที่ทุกเส้นทางถูกลงทะเบียน:

- เส้นทางสาธารณะ (`/healthz`, `/auth/login`, `/auth/refresh`, ...);
- เส้นทางแผงที่ยืนยันตัวตน (`/api/v1/*`) อยู่หลัง `authMw + audit + accesslog`;
- เส้นทางเขียนได้รับการป้องกันเพิ่มเติมด้วย `permMw("permission.key")`;
- เส้นทาง agent (`/agent/v1/*`) อยู่หลัง `middleware.AgentAuth`;
- เส้นทาง WebSocket (`/terminal`, `/containers/:id/exec`)

**ข้อตกลง:** ทุกเอ็นด์พอยต์เขียน (POST/PUT/DELETE) ต้องมีการ์ด `permMw(...)` เอ็นด์พอยต์อ่านเปิดให้ผู้ใช้ที่เข้าสู่ระบบทุกคน เว้นแต่จะเปิดเผยข้อมูลละเอียดอ่อน

### 4.4 `internal/middleware`

| ไฟล์ | วัตถุประสงค์ |
|------|--------------|
| `auth.go` | การยืนยันตัวตน JWT; ฉีด id/role ของผู้ใช้ลงในคอนเท็กซ์ |
| `perm.go` | ตรวจสอบสิทธิ์ RBAC (`permMw`) |
| `audit.go` | เขียนรายการบันทึกการตรวจสอบ |
| `accesslog.go` | เขียนรายการบันทึกการเข้าถึง |
| `cors.go` | ส่วนหัว CORS |
| `ratelimit.go` | จำกัดอัตราการเข้าสู่ระบบ |
| `metrics_auth.go` | การ์ด Bearer Token สำหรับ `/metrics` |
| `agent.go` | การยืนยันตัวตน Agent Token สำหรับ `/agent/v1` |

### 4.5 `internal/api/v1`

หนึ่งไฟล์ handler ต่อโมดูล แต่ละ handler:

1. ผูก/ตรวจสอบคำขอ;
2. เรียกเมธอด service ที่สอดคล้อง;
3. คืน `response.OK` / `response.Error` แบบรวม

handler ของแผงอยู่ในแพ็กเกจ `v1` (เช่น `system.go`, `file.go`, `skill.go`); handler ของ agent อยู่ใน `agent.go`

### 4.6 `internal/service`

เลเยอร์ตรรกะธุรกิจ หนึ่งไฟล์ต่อโมดูล โมดูลที่สำคัญ:

| ไฟล์ | โมดูล |
|------|-------|
| `auth.go`, `user.go`, `role.go`, `totp.go` | การยืนยันตัวตน, ผู้ใช้, RBAC, 2FA |
| `system.go`, `metrics.go`, `prometheus.go` | ข้อมูลโฮสต์, เมตริก, การส่งออก prometheus |
| `website.go`, `database.go`, `cron.go`, `crontab.go` | การจัดการทรัพยากร |
| `file.go` | การดำเนินการไฟล์พร้อมการป้องกัน path-traversal |
| `docker.go` | คอนเทนเนอร์/อิมเมจ/วอลุ่ม/เน็ตเวิร์ก Docker |
| `ai.go` | อะแดปเตอร์ LLM (แชท, สตรีมมิง) |
| `skill.go`, `mcp.go` | สกิล AI + เซิร์ฟเวอร์ MCP |
| `alert.go`, `alertmonitor.go` | กฎการแจ้งเตือน + การประเมิน |
| `security.go`, `securitymonitor.go`, `baseline.go`, `fim.go`, `threat.go`, `firewall.go`, `loginsecurity.go` | ชุดความปลอดภัยโฮสต์ |
| `notification.go`, `audit.go`, `setting.go` | การแจ้งเตือน, การตรวจสอบ, การตั้งค่า |
| `appstore.go`, `apptemplate.go`, `appcategory.go` | ร้านแอป / เทมเพลต |

### 4.7 `internal/repository`

การเข้าถึงข้อมูล GORM หนึ่งไฟล์ต่อโมเดล ให้ CRUD และตัวช่วยค้นหา ไม่มีตรรกะธุรกิจ

### 4.8 `internal/model`

GORM struct ที่แมปกับตาราง SQLite บวกกับ `role.go` ซึ่งเป็นแหล่งเดียวที่เชื่อถือได้ของกลุ่มสิทธิ์ RBAC (`PermGroups()`, `AllPermKeys()`, `BuiltinRoles()`)

### 4.9 `internal/pkg`

| แพ็กเกจ | วัตถุประสงค์ |
|---------|--------------|
| `jwt` | การลงนามและตรวจสอบ access/refresh token |
| `response` | ซองการตอบกลับแบบรวม `{code,message,data}` |
| `store` | การเปิด SQLite, การย้ายสคีมาอัตโนมัติ, seed (admin เริ่มต้น) |

### 4.10 `web`

- `index.html` — Vue3 SPA ไฟล์เดียว: i18n 7 ภาษา, ระบบธีม, เข้าสู่ระบบ, แดชบอร์ด, การตรวจสอบ, ความปลอดภัย, ไฟล์, เทอร์มินัล, ผู้ช่วย AI, การตั้งค่า
- `embed.go` — `go:embed` ฝังฟรอนต์เอนด์ลงในไบนารี
- `static/` — ดีเพนเดนซีฟรอนต์เอนด์แบบออฟไลน์ (Vue, ECharts)

---

## 5. การออกแบบ API

### 5.1 ซองการตอบกลับ

ทุกเอ็นด์พอยต์คืน:

```json
{ "code": 0, "message": "ok", "data": { } }
```

- `code == 0` → สำเร็จ, `data` บรรจุเพย์โหลด;
- `code != 0` → ข้อผิดพลาดทางธุรกิจ, `message` อธิบาย

### 5.2 การยืนยันตัวตน

- API แผง (`/api/v1`): JWT access token (`Authorization: Bearer <token>`) อายุสั้น รีเฟรชผ่าน `/auth/refresh`
- Agent API (`/agent/v1`): Agent Token แบบสแตติก (คอนฟิก `agent.token`)

### 5.3 กลุ่มเอ็นด์พอยต์

| กลุ่ม | ผู้ใช้ | การยืนยันตัวตน |
|------|-------|-----------------|
| `/api/v1/*` | UI เบราว์เซอร์ | JWT ผู้ใช้ + RBAC |
| `/agent/v1/*` | ระบบภายนอก / พร็อกซี | Agent Token |

---

## 6. โมเดลข้อมูล

โมเดลคือ GORM struct ใน `internal/model` ตารางถูกย้ายสคีมาอัตโนมัติตอนเริ่มต้นโดย `internal/pkg/store` โมเดลตัวแทน:

- `User` (id, username, แฮชรหัสผ่าน, role, ซีเคร็ต MFA, ...)
- `Role` (name, label, perms, builtin)
- `Website`, `Database`, `CronJob`
- `AlertRule`, `AlertEvent`, `Notification`
- `McpServer`, `Skill` (สกิลเก็บเป็นไดเรกทอรีบนดิสก์ ไม่ใช่ DB)
- `AuditLog`, `AccessLog`, `Setting`
- ความปลอดภัย: `BaselineResult`, `FimBaseline`, `FimChange`, `ThreatFinding`

---

## 7. RBAC และสิทธิ์

กลุ่มสิทธิ์ถูกกำหนดใน `internal/model/role.go` (`PermGroups()`) ซึ่งเป็นแหล่งข้อมูลเดียว สามบทบาทในตัว:

- **admin** — สิทธิ์ `"*"` (ทั้งหมด);
- **operator** — สิทธิ์ทั้งหมดยกเว้น `user.*` และ `settings.edit`;
- **readonly** — สิทธิ์ `*.view` เท่านั้น

เมนู/ปุ่มฟรอนต์เอนด์ควบคุมด้วยคีย์เดียวกันผ่าน `hasPerm('key')`; แบ็กเอนด์บังคับผ่าน `permMw("key")` เมื่อเพิ่มฟีเจอร์เขียนใหม่ คุณ**ต้อง**ทำทั้งสามอย่าง:

1. เพิ่มคีย์สิทธิ์ลงใน `PermGroups()`;
2. ป้องกันเส้นทางด้วย `permMw(...)`;
3. ป้องกันปุ่มด้วย `hasPerm(...)`

---

## 8. การบิลด์และการปรับใช้

```bash
make build          # บิลด์สำหรับแพลตฟอร์มปัจจุบัน
make build-all      # คอมไพล์ข้ามทุกแพลตฟอร์ม
make version        # พิมพ์ข้อมูลเวอร์ชัน
```

ไบนารีเป็นสแตติก (ไม่มี CGO) ดู [`build-and-deploy.md`](build-and-deploy.md) สำหรับคำแนะนำ systemd, reverse proxy และการอัปเกรด

---

## 9. คู่มือการพัฒนา

### 9.1 การเพิ่มโมดูลใหม่

ทำตามรูปแบบเลเยอร์ — สร้าง (หรือขยาย):

1. `internal/model/xxx.go` — GORM struct;
2. `internal/repository/xxx.go` — การเข้าถึงข้อมูล;
3. `internal/service/xxx.go` — ตรรกะธุรกิจ;
4. `internal/api/v1/xxx.go` — handler;
5. ลงทะเบียนเส้นทางใน `internal/router/router.go`

### 9.2 การเพิ่มเอ็นด์พอยต์เขียนใหม่

1. เพิ่มคีย์สิทธิ์ลงใน `internal/model/role.go`;
2. ลงทะเบียนเส้นทางด้วย `permMw("...")`;
3. เพิ่มการ์ด `hasPerm("...")` ให้ปุ่มฟรอนต์เอนด์;
4. เพิ่มคีย์ i18n (ครบ 7 ภาษา) ใน `web/index.html`

### 9.3 ข้อตกลง i18n

พจนานุกรม i18n ฟรอนต์เอนด์ (`I18N`...`I18N8`) บรรจุ 7 ภาษา: `zh-CN`, `zh-TW`, `en`, `ja`, `ko`, `th`, `de` ทุกคีย์ใหม่ต้องเพิ่มครบ**ทั้ง 7 ภาษา** — ตัวช่วย `t(key)` จะย้อนกลับไป `zh-CN` เมื่อขาด แต่การแปลที่ขาดจะแสดงภาษาจีนให้ผู้ใช้ที่ไม่ใช่จีน

### 9.4 สไตล์โค้ด

- ฟังก์ชัน/ไทป์ Go ที่ส่งออกมีคอมเมนต์เอกสารที่ขึ้นต้นด้วยชื่อของมัน
- คอมเมนต์เขียนเป็นภาษาอังกฤษ
- ทุกไดเรกทอรีมี `README.md` อธิบายไฟล์ของมัน
