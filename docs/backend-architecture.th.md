<div align="center">

# OpsMini Backend Architecture

**การออกแบบสถาปัตยกรรมแบ็กเอนด์**

[English](backend-architecture.md) · [简体中文](backend-architecture.zh-CN.md) · [繁體中文](backend-architecture.zh-TW.md) · [日本語](backend-architecture.ja.md) · [한국어](backend-architecture.ko.md) · [ไทย](backend-architecture.th.md) · [Deutsch](backend-architecture.de.md)

</div>

---

> เวอร์ชัน：v1.0  
> อ้างอิง：ต้นแบบ UI `web/index.html`（เข้าสู่ระบบ/ผู้ใช้/แดชบอร์ด/การตรวจสอบ/แอป/คอนเทนเนอร์/ระบบ/ไฟล์/เทอร์มินัล/งานตามกำหนดเวลา/บันทึก/ผู้ช่วย AI/การตั้งค่าแผง/i18n 7 ภาษา）

---

## 1. ภาพรวม

OpsMini เป็น**แผงควบคุมการดำเนินงานเครื่องเดียวแบบเบา**（เทียบเท่า BaoTa / 1Panel）ที่มีจุดแตกต่างหลักสองประการ：

1. **โมเดล AI ขนาดใหญ่ในตัว**สำหรับการจัดการระบบ（การวินิจฉัย / การดำเนินการ / การวิเคราะห์บันทึกด้วยภาษาธรรมชาติ）；
2. **เปิดเผย REST API มาตรฐาน**ให้ระบบภายนอก（แพลตฟอร์มตรวจสอบ สคริปต์อัตโนมัติ เครื่องมือออร์เคสตราของบุคคลที่สาม）เรียกใช้งานได้

> หมายเหตุขอบเขต v1.0：เวอร์ชันนี้**ไม่รวมโหนดศูนย์กลาง Proxy** แผง OpsMini บนแต่ละโฮสต์ทำงานอย่างอิสระ
> และเปิดเผยความสามารถผ่าน REST API มาตรฐาน การจัดการหลายเครื่องแบบรวมศูนย์（Proxy）จะประเมินในเวอร์ชันถัดไป

### 1.1 เป้าหมายการออกแบบ

| เป้าหมาย | คำอธิบาย |
|------|------|
| เบา | การปรับใช้ไบนารีเดียว ใช้หน่วยความจำน้อย เหมาะกับโฮสต์ขนาดเล็ก 1C1G |
| เครื่องเดียวก่อน | สถานการณ์หลักคือเซิร์ฟเวอร์เครื่องเดียว ไม่เพิ่มความซับซ้อนแบบกระจาย |
| บูรณาการได้ | เปิดเผยความสามารถผ่าน REST API มาตรฐานเพื่อให้ระบบภายนอกบูรณาการ |
| ปลอดภัย | RBAC, 2FA, ทางเข้าที่ปลอดภัย, สิทธิ์น้อยที่สุด, การเข้ารหัสคีย์ |
| บำรุงรักษาง่าย | แบ่งชั้นแบบโมดูล Controller → Service → Repository ชัดเจน |

### 1.2 การเลือกเทคโนโลยี

| ชั้น | ตัวเลือก | ทางเลือก | เหตุผล |
|----|------|------|------|
| ภาษา | Go 1.22+ | — | ไบนารีเดียว คอมไพล์ข้ามแพลตฟอร์ม พร้อมกันดี ระบบนิเวศที่สมบูรณ์（สแต็กเดียวกับ 1Panel） |
| เฟรมเวิร์กเว็บ | **Gin** | Echo / chi | ระบบนิเวศใหญ่ที่สุด มิดเดิลแวร์หลากหลาย ตัวเดียวกับ 1Panel |
| ฐานข้อมูล | **SQLite** | — | ฝังตัว ศูนย์การดำเนินงาน เหมาะกับสถานการณ์เครื่องเดียว |
| ไดรเวอร์ SQLite | **modernc.org/sqlite** | mattn/go-sqlite3 | Pure Go ไม่มี CGO คอมไพล์ข้ามแพลตฟอร์มง่าย |
| ORM | **GORM** | sqlx | ประสิทธิภาพการพัฒนาสูง คิวรีซับซ้อนกลับไปใช้ SQL ดิบได้ |
| สื่อสารเรียลไทม์ | **gorilla/websocket** | — | เทอร์มินัล, tail บันทึก, การพุชเมตริก |
| การจัดตารางงาน | **robfig/cron** | — | งานตามกำหนดเวลาของแผง |
| ตรวจสอบระบบ | **gopsutil** | อ่าน /proc | CPU/หน่วยความจำ/ดิสก์/โปรเซสข้ามแพลตฟอร์ม |
| Docker | SDK ทางการ | — | คอนเทนเนอร์/อิมเมจ/โวลุ่ม/เครือข่าย |
| บันทึก | zerolog | zap | เบา มีโครงสร้าง การจัดสรรต่ำ |
| การรับรอง | JWT + refresh | session | API แบบไร้สถานะ + เซสชันทางเลือก |
| 2FA | TOTP（RFC 6238） | — | ใช้ซ้ำ pquerna/otp |
| AI | อินเทอร์เฟซ LLM แบบนามธรรม | — | อะแดปเตอร์รวม OpenAI/DeepSeek/Qwen/Ollama |

### 1.3 สถาปัตยกรรมโดยรวม

```
┌────────────────────────────────────────────────────────────┐
│                OpsMini（单机面板，单二进制）                    │
│                                                            │
│   Vue3 SPA（构建产物 embed 进二进制）                          │
│        │  HTTP / WebSocket                                  │
│   ┌────▼────────────────────────────────────────────────┐   │
│   │  Gin Router → 中间件（认证/权限/审计/限流/安全入口）     │   │
│   │  Controller（参数校验）→ Service（业务）→ Repo（GORM）  │   │
│   └────┬────────────────────────────────────────────────┘   │
│        │                                                    │
│   ┌────▼────────────────────────────────────────────────┐   │
│   │  SQLite（业务数据）   │   系统资源适配层                │   │
│   │  users/cron/websites │   Docker SDK / crontab /       │   │
│   │  ...                 │   gopsutil / 文件系统 / SSH    │   │
│   └──────────────────────┴───────────────────────────────┘   │
│                                                              │
│   对外暴露两套标准 REST API：                                   │
│   · /api/v1   面板 API（浏览器，用户 JWT + RBAC）              │
│   · /agent/v1 Agent API（机器，Agent Token，命令白名单）       │
└──────────────────────────────────────────────────────────────┘
```

---

## 2. การตัดสินใจสถาปัตยกรรมหลัก

### 2.1 โมโนลิธไบนารีเดียว ไม่ทำไมโครเซอร์วิส

**การตัดสินใจ**：โมโนลิธกระบวนการเดียว ผลลัพธ์การสร้าง Vue3 ฝังลงในไบนารีผ่าน `go:embed` ส่งมอบไฟล์ปฏิบัติการ `opsmini` ไฟล์เดียว

**เหตุผล**：
- ความต้องการหลักของแผงดำเนินงานคือ "ติดตั้งหนึ่งเครื่อง จัดการหนึ่งเครื่อง" โมโนลิธเหมาะสมที่สุด；
- การบูรณาการภายนอกผ่าน REST API มาตรฐาน ไม่ขัดแย้งกับโมโนลิธ；
- ไมโครเซอร์วิสจะนำความซับซ้อนที่ไม่จำเป็น เช่น การปรับใช้ การค้นพบบริการ ธุรกรรมแบบกระจาย

### 2.2 แยกหน้า/หลัง + แพ็กเกจฝังตัว

- ช่วงพัฒนา：Vue3 dev server พร็อกซีไปยัง Gin（CORS / reverse proxy）；
- ช่วงใช้งานจริง：ผลลัพธ์ `web/dist` ฝังลงไบนารีด้วย `go:embed` ปรับใช้ไฟล์เดียว

### 2.3 SQLite แทน MySQL/Postgres

- ปริมาณข้อมูลธุรกิจน้อย（การตั้งค่า งาน ผู้ใช้ บันทึก）SQLite เพียงพอ；
- ศูนย์การดำเนินงาน ไฟล์เดียว สำรองง่าย（คัดลอก .db โดยตรง）；
- หากข้อมูลของแผงต้องการความพร้อมกันสูงในอนาคต สามารถแยกอินเทอร์เฟซ Repository เพื่อสลับ

### 2.4 เปิดเผย REST API มาตรฐาน

- v1.0 **ไม่พัฒนาโหนดศูนย์กลาง Proxy** เพียงเปิดเผยความสามารถของแผงเป็น REST API มาตรฐาน；
- สอง API อยู่ร่วมกัน：`/api/v1`（สำหรับ UI เบราว์เซอร์）และ `/agent/v1`（สำหรับเครื่อง/การบูรณาการบุคคลที่สาม）；
- Agent API ใช้การรับรองอิสระ（Agent Token）+ รายการอนุญาตคำสั่ง แยกจาก JWT เซสชันผู้ใช้；
- หากต้องการจัดการหลายเครื่องรวมศูนย์ในอนาคต เพิ่มชั้น Proxy ดึง/จัดตารางบน Agent API ได้（นอกขอบเขต v1.0）

---

## 3. การแบ่งโมดูล（สอดคล้องกับต้นแบบ UI）

| โมดูลแบ็กเอนด์ | หน้าที่ | หน้าต้นแบบที่สอดคล้อง |
|----------|------|-------------|
| `auth` | เข้าสู่ระบบ/ออกจากระบบ เซสชัน 2FA RBAC | หน้าเข้าสู่ระบบ จัดการผู้ใช้ |
| `setting` | การตั้งค่าแผง ธีม การแสดงเมนู ภาษา | การตั้งค่าแผง（พื้นฐาน/ลักษณะ/เมนู） |
| `dashboard` | รวมเมตริก พุชเรียลไทม์ | แดชบอร์ด |
| `monitor` | เก็บอนุกรมเวลา กฎการแจ้งเตือน การทริกเกอร์ | การตรวจสอบ |
| `website` | ไซต์ Nginx โดเมน ใบรับรอง SSL | จัดการแอป-เว็บไซต์ |
| `database` | อินสแตนซ์และฐานข้อมูล MySQL/PostgreSQL | จัดการแอป-ฐานข้อมูล |
| `store` | ติดตั้ง/ถอน/อัปเกรดซอฟต์แวร์ | จัดการแอป-ร้านซอฟต์แวร์ |
| `container` | คอนเทนเนอร์/อิมเมจ/โวลุ่ม/เครือข่าย Docker | จัดการคอนเทนเนอร์ |
| `system` | โปรเซส/เครือข่าย/พอร์ต/ดิสก์ | จัดการระบบ |
| `file` | เรียกดู/อัปโหลด/แก้ไข/สิทธิ์ไฟล์ | ไฟล์ |
| `terminal` | Web SSH | เทอร์มินัล |
| `cron` | งานตามกำหนดเวลา（แผง+ระบบ+ผู้ใช้ crontab） | งานตามกำหนดเวลา |
| `log` | เก็บ/รวม/tail บันทึก | บันทึก |
| `ai` | การเชื่อมต่อ LLM บริบท การดำเนินการ NL | ผู้ช่วย AI, การตั้งค่าแผง-AI |
| `agent` | REST API ภายนอก（`/agent/v1`） การรับรอง Agent Token รายการอนุญาตคำสั่ง | การตั้งค่าแผง-การเชื่อมต่อ Proxy |

---

## 4. การแบ่งชั้นและโครงสร้างไดเรกทอรี

### 4.1 การแบ่งชั้น

ภายในแต่ละโมดูลแบ่งสามชั้นอย่างเคร่งครัด：

```
Controller（HTTP handler）：参数解析校验、调用 Service、组装响应
    → Service（业务逻辑）：编排 Repository 与系统资源、事务边界
    → Repository（GORM）：纯数据访问，不写业务
```

### 4.2 โครงสร้างไดเรกทอรี

```
opsmini/
├── cmd/
│   └── agent/main.go          # 单机面板主程序
├── internal/
│   ├── router/                # 路由注册、API 版本
│   ├── middleware/            # 认证、权限、审计、限流、安全入口
│   ├── api/v1/                # Controller（按模块分包，含 /api/v1 与 /agent/v1）
│   ├── service/               # Service（按模块分包）
│   ├── repository/            # Repository（按模块分包）
│   ├── model/                 # GORM 数据模型
│   ├── agent/                 # Agent REST API（对外暴露、token 认证、命令白名单）
│   ├── ai/                    # LLM 适配层（provider 接口 + 各实现）
│   └── pkg/                   # 通用工具
│       ├── config/            # 配置加载（文件+环境变量）
│       ├── logger/            # zerolog 封装
│       ├── jwt/               # token 签发/校验
│       ├── otp/               # 2FA TOTP
│       ├── sysinfo/           # gopsutil 封装（采集指标）
│       ├── crontab/           # 系统/用户 crontab 读写
│       ├── docker/            # Docker SDK 封装
│       └── store/             # SQLite 连接 + 迁移
├── web/                       # Vue3 前端源码（构建后 embed）
├── docs/
├── configs/                   # 默认配置示例
└── go.mod
```

---

## 5. โมเดลข้อมูล（สคีมา SQLite）

> การย้าย GORM；ฟิลด์ที่ละเอียดอ่อน（คีย์/Token）เข้ารหัสด้วย AES-GCM ก่อนจัดเก็บ

```sql
-- 用户与认证
CREATE TABLE users (
  id            INTEGER PRIMARY KEY AUTOINCREMENT,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,          -- bcrypt
  role          TEXT NOT NULL DEFAULT 'operator',  -- admin/operator/readonly
  auth_method   TEXT NOT NULL DEFAULT 'password',  -- password/2fa
  totp_secret   TEXT,                    -- 加密存储
  status        INTEGER NOT NULL DEFAULT 1,        -- 1启用 0停用
  last_login    TEXT,
  created_at    TEXT,
  updated_at    TEXT
);

CREATE TABLE sessions (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER NOT NULL,
  refresh    TEXT NOT NULL UNIQUE,
  expire_at  TEXT NOT NULL,
  created_at TEXT
);

-- 面板配置（KV，含主题/语言/菜单显隐/代理）
CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value TEXT
);

-- 网站
CREATE TABLE websites (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  domain     TEXT NOT NULL UNIQUE,
  path       TEXT NOT NULL,
  env        TEXT,                       -- nginx/static
  runtime    TEXT,                       -- php8.2/php8.1/node20/static
  ssl        INTEGER DEFAULT 0,
  ssl_days   INTEGER,
  status     INTEGER DEFAULT 1,
  created_at TEXT
);

-- 数据库实例
CREATE TABLE databases (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  type      TEXT NOT NULL,               -- mysql/postgresql
  name      TEXT NOT NULL,
  charset   TEXT,
  created_at TEXT
);

-- 计划任务（含系统/用户 crontab 的只读映射）
CREATE TABLE cron_jobs (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  name      TEXT NOT NULL,
  kind      TEXT NOT NULL,               -- panel/system/user
  user      TEXT,
  src_path  TEXT,                        -- /etc/crontab 或 sqlite 等
  schedule  TEXT NOT NULL,               -- cron 表达式
  command   TEXT NOT NULL,
  enabled   INTEGER DEFAULT 1,
  last_run  TEXT,
  created_at TEXT
);

-- 告警规则
CREATE TABLE alert_rules (
  id        INTEGER PRIMARY KEY AUTOINCREMENT,
  name      TEXT NOT NULL,
  metric    TEXT NOT NULL,               -- cpu/mem/disk/service
  condition TEXT NOT NULL,               -- >90% 等
  duration  TEXT,
  notify    TEXT,
  enabled   INTEGER DEFAULT 1,
  created_at TEXT
);

-- Agent API 配置
CREATE TABLE agent_config (
  id        INTEGER PRIMARY KEY CHECK (id = 1),  -- 单行
  agent_id  TEXT,
  token     TEXT,                        -- 加密存储（对外 REST API 认证）
  allowed_commands TEXT,                 -- 命令白名单（逗号分隔）
  enabled   INTEGER DEFAULT 0
);

-- 操作/审计日志
CREATE TABLE audit_logs (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id    INTEGER,
  action     TEXT,
  target     TEXT,
  detail     TEXT,
  created_at TEXT
);
```

---

## 6. การออกแบบ API（RESTful, `/api/v1`）

การตอบกลับรวม：`{ "code": 0, "message": "ok", "data": ... }`，`code` ที่ไม่ใช่ 0 คือข้อผิดพลาดทางธุรกิจ

| เมธอด | พาธ | คำอธิบาย |
|------|------|------|
| POST | `/auth/login` | เข้าสู่ระบบ（คืน access + refresh） |
| POST | `/auth/refresh` | รีเฟรช token |
| POST | `/auth/logout` | ออกจากระบบ |
| GET | `/auth/2fa/qrcode` | สร้างคิวอาร์โค้ด 2FA |
| POST | `/auth/2fa/verify` | ตรวจสอบ 2FA |
| GET | `/users` / POST / PUT / DELETE | CRUD ผู้ใช้ |
| GET | `/dashboard/metrics` | เมตริกแดชบอร์ด |
| GET | `/monitor/series?range=1h` | ข้อมูลอนุกรมเวลา |
| CRUD | `/alert-rules` | กฎการแจ้งเตือน |
| CRUD | `/websites` | เว็บไซต์ |
| POST | `/websites/:id/ssl` | ออก/ต่ออายุ SSL |
| CRUD | `/databases` | ฐานข้อมูล |
| GET | `/store/apps` / POST `/store/apps/:id/install` | ร้านซอฟต์แวร์ |
| GET | `/containers` / `/images` / `/volumes` / `/networks` | คอนเทนเนอร์สี่ประเภท |
| POST | `/containers` เป็นต้น | สร้างคอนเทนเนอร์/ดึงอิมเมจ/สร้างโวลุ่ม/สร้างเครือข่าย |
| GET | `/system/processes` `/networks` `/ports` `/disks` | ทรัพยากรระบบ |
| GET/POST | `/files` / `/files/list` / `/files/upload` / `/files/edit` | ไฟล์ |
| WS  | `/terminal/ws?cols=&rows=` | Web SSH |
| CRUD | `/cron-jobs` | งานตามกำหนดเวลา（ประเภทแผง） |
| GET | `/cron-jobs/system` `/cron-jobs/user` | crontab ระบบ/ผู้ใช้แบบอ่านอย่างเดียว |
| GET | `/logs` | รายการบันทึก |
| POST | `/ai/chat` | สนทนา AI（สตรีม SSE） |
| POST | `/ai/execute` | แปลง NL เป็นการดำเนินการ（พร้อมยืนยันสิทธิ์） |
| GET/PUT | `/agent/config` | การตั้งค่า Agent API（token, รายการอนุญาตคำสั่ง） |
| GET/PUT | `/settings` | การตั้งค่าแผง |
| GET | `/i18n/{lang}` | แพ็กภาษา（หน้าเว็บก็ฝังได้） |
| GET | `/metrics` | **เมตริก Prometheus**（เข้ากันได้กับ node_exporter ไม่มีคำนำหน้า `/api/v1`） |

### 6.1 การบูรณาการการตรวจสอบ Prometheus

OpsMini มี**เอนด์พอยต์ `/metrics` ที่เข้ากันได้กับ node_exporter** ในตัว ไม่ต้องติดตั้ง node_exporter เพิ่ม Prometheus สามารถสแครปได้โดยตรง：

```
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
```

จัดแนวเมตริกหลักของ node_exporter แล้ว（ใช้ Node Dashboard ชุมชนได้โดยตรง）：

| ตระกูลเมตริก | คำอธิบาย |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | วินาทีสะสมต่อคอร์ต่อโหมด |
| `node_memory_MemTotal_bytes` เป็นต้น | หน่วยความจำ Total/Free/Available/Buffers/Cached |
| `node_filesystem_size_bytes{mountpoint}` | ความจุ/ว่าง/อัตราการใช้ต่อจุดเมานต์ |
| `node_network_receive_bytes_total{device}` | ปริมาณรับส่งต่อการ์ดเครือข่าย |
| `node_load1` / `node_load5` / `node_load15` | โหลด |
| `node_uname_info` / `node_boot_time_seconds` | ข้อมูลโฮสต์และเวลาเริ่มระบบ |

> วิธีดำเนินการ：ใช้ซ้ำ `gopsutil`（ข้อมูลที่ SystemService เก็บแล้ว）ส่งออกเป็นรูปแบบข้อความ Prometheus
> คงการส่งมอบไบนารีเดียว ไม่ฝังกระบวนการ node_exporter

### 6.2 Agent API（อินเทอร์เฟซ REST มาตรฐานภายนอก）

นอกเหนือจาก `/api/v1` ที่ UI ใช้ แผงยังเปิดเผย**REST API แบบเครื่องต่อเครื่องอิสระ**（`/agent/v1`）ให้ระบบภายนอก（แพลตฟอร์มตรวจสอบ สคริปต์อัตโนมัติ เครื่องมือออร์เคสตราบุคคลที่สาม）บูรณาการ ความแตกต่างจาก API แผง：

- **การรับรอง**：ใช้ Agent Token ไม่ใช่ JWT เซสชันผู้ใช้；
- **ขอบเขต**：เน้นการจัดการทรัพยากรและการดำเนินการคำสั่ง ไม่รวมความสามารถเฉพาะ UI（i18n/ธีม/เมนู）และเทอร์มินัล WS；
- **สไตล์**：เน้นระบบอัตโนมัติ——ไอเดมโพเทนต์ ลองใหม่ได้ ตอบกลับ JSON รวม

| เมธอด | พาธ | คำอธิบาย |
|------|------|------|
| GET | `/agent/v1/health` | ตรวจสุขภาพ（ตรวจการมีชีวิต） |
| GET | `/agent/v1/status` | สรุปสถานะ：cpu/mem/ดิสก์/บริการออนไลน์ |
| GET | `/agent/v1/system/info` | ข้อมูลโฮสต์（hostname/os/เคอร์เนล） |
| GET | `/agent/v1/system/processes` | รายการโปรเซส |
| GET | `/agent/v1/system/ports` | พอร์ตที่รอรับ |
| GET | `/agent/v1/system/disks` | ดิสก์/จุดเมานต์ |
| GET | `/agent/v1/websites` / POST | ค้นหา/สร้างเว็บไซต์ |
| GET | `/agent/v1/databases` | รายการฐานข้อมูล |
| GET | `/agent/v1/containers` | รายการคอนเทนเนอร์ |
| GET | `/agent/v1/cron-jobs` | งานตามกำหนดเวลา |
| POST | `/agent/v1/commands` | ดำเนินการคำสั่ง（รายการอนุญาต คืนผลลัพธ์การดำเนินการ） |

---

## 7. การออกแบบ REST API ภายนอก（จุดเน้นความแตกต่าง）

### 7.1 การวางตำแหน่ง

OpsMini v1.0 เป็น**แผงเครื่องเดียว** เปิดเผยความสามารถผ่าน REST API มาตรฐานเพื่อให้ระบบภายนอกบูรณาการ：

- **`/api/v1`（API แผง）**：สำหรับ UI เบราว์เซอร์ JWT เซสชันผู้ใช้ + RBAC；
- **`/agent/v1`（Agent API）**：สำหรับเครื่อง/การบูรณาการบุคคลที่สาม การรับรอง Agent Token + รายการอนุญาตคำสั่ง

> ต่างจาก "การพุชการเชื่อมต่อยาวขาออก" ของ salt minion/master OpsMini **เปิดเผย REST ให้ภายนอกดึงเรียก**โดยตรง
> ใกล้เคียงแนวคิด Prometheus ดึง exporter / OpenAPI ของผู้ให้บริการคลาวด์ การนำ Proxy ศูนย์กลางมาบริหารหลายเครื่องรวมศูนย์จะประเมินในเวอร์ชันถัดไป（ไม่ทำใน v1.0）

### 7.2 โมเดลการเรียกใช้

```
         HTTPS REST 调用（Agent Token）
   ┌──────────┐  ─────────────────────▶  ┌─────────┐
   │ 外部系统  │                          │ OpsMini │
   │ (监控/脚本 │  ◀─────────────────────  │ (单机面板)│
   │ /编排工具) │       统一 JSON 响应      └─────────┘
   └──────────┘
```

- **ไม่มีการเชื่อมต่อยาว**：ทั้งหมดใช้ REST มาตรฐาน ไม่คงการเชื่อมต่อยาว WebSocket / gRPC；
- **สถานการณ์สตรีม**（tail บันทึก เมตริกเรียลไทม์）：โพลแบบแบ่งหน้า REST ก็พอ ไม่ต้อง SSE；
- **ไอเดมโพเทนต์**：GET แบบคิวรีไอเดมโพเทนต์โดยธรรมชาติ；การเขียน（ดำเนินการคำสั่ง สร้างทรัพยากร）คืนผลลัพธ์ชัดเจน

### 7.3 กระบวนการสำคัญ

1. แผงเริ่มทำงาน → อ่าน `agent_config`（token + รายการอนุญาตคำสั่ง）；
2. ระบบภายนอกเรียก `/agent/v1/*` พร้อม `Authorization: Bearer <token>`；
3. มิดเดิลแวร์ตรวจสอบ token（เปรียบเทียบเวลาคงที่）→ ตรงรายการอนุญาตจึงผ่าน มิฉะนั้น 401；
4. การดำเนินการเสี่ยงสูง（ดำเนินการคำสั่ง）ผ่านการตรวจสอบรายการอนุญาตคำสั่งครั้งที่สอง บันทึกการตรวจสอบทั้งหมด；
5. คืน JSON รวม：`{ code, message, data }`

### 7.4 ความปลอดภัย

- **การรับรอง**：แผงตั้งค่า Agent Token สุ่มอิสระ（เก็บเข้ารหัส）ส่วนหัว `Authorization: Bearer <token>`；
- **การส่ง**：HTTPS；สถานการณ์ความปลอดภัยสูงเพิ่มรายการอนุญาต IP ได้；
- **รายการอนุญาตคำสั่ง**：`/commands` อนุญาตเฉพาะคำสั่งที่อนุญาตชัดเจน บันทึกการตรวจสอบทั้งหมด；
- **การเปิดเผยน้อยที่สุด**：`/agent/v1` แยกจาก `/api/v1` Agent API ไม่เปิดเผยความสามารถ UI และเทอร์มินัล

### 7.5 การวางตำแหน่งของสอง API

| มิติ | API แผง `/api/v1` | Agent API `/agent/v1` |
|------|-------------------|----------------------|
| ผู้ใช้ | เบราว์เซอร์（คน） | ระบบภายนอก（เครื่อง） |
| การรับรอง | JWT เซสชันผู้ใช้ + RBAC | Agent Token |
| ขอบเขต | ฟังก์ชัน UI ทั้งหมด + เทอร์มินัล WS | จัดการทรัพยากร + ดำเนินการคำสั่ง（ไม่มี UI/เทอร์มินัล） |
| การออกแบบ | เน้นการโต้ตอบ | เน้นระบบอัตโนมัติ（ไอเดมโพเทนต์ ลองใหม่ได้） |

---

## 8. การออกแบบความปลอดภัย

| มิติ | แนวทาง |
|------|------|
| รหัสผ่าน | แฮช bcrypt |
| เซสชัน | access JWT ระยะสั้น（15 นาที）+ refresh token（เพิกถอนได้） |
| 2FA | TOTP（RFC 6238）ตรวจสอบครั้งที่สองทางเลือกเมื่อเข้าสู่ระบบ |
| การอนุญาต | RBAC สามบทบาท：admin（ทั้งหมด）/ operator（ดำเนินงานประจำวัน）/ readonly（อ่านอย่างเดียว） |
| ทางเข้าที่ปลอดภัย | เข้าถึงแผงต้องมีพาธลับ（เช่น `/opsmini_panel`）ป้องกันการสแกนพอร์ต |
| การเก็บคีย์ | คีย์แผง（API Key, Agent Token）เก็บเข้ารหัส AES-GCM |
| เทอร์มินัล | Web SSH เปิดเฉพาะ operator ขึ้นไป บันทึกเซสชันตรวจสอบ |
| ป้องกันการเดาสุ่ม | จำกัดอัตราเมื่อเข้าสู่ระบบล้มเหลว + ล็อก |
| การตรวจสอบ | การดำเนินการสำคัญเขียน `audit_logs` |
| CSRF/XSS | API ใช้ Bearer Token ไม่มีคุกกี้；หน้าเว็บ escape |

---

## 9. กระบวนการสำคัญ

### 9.1 เข้าสู่ระบบ

```
输入账密 → bcrypt 校验 → 若开启 2FA 则要求 TOTP
  → 签发 access + refresh → 前端存 refresh（httpOnly/localStorage）
  → 后续请求带 Bearer access → 过期用 refresh 换新
```

### 9.2 เทอร์มินัล Web SSH

```
前端 ws://host/api/v1/terminal/ws?token=...
  → 服务端校验 token + 角色
  → 启动 pty（github.com/creack/pty）→ 双向数据转发
  → 关闭时回收 pty、记录会话时长
```

### 9.3 การดำเนินการงานตามกำหนดเวลา

- **งานแผง**：robfig/cron จัดตารางประจำ เขียน `cron_jobs`；
- **งานระบบ/ผู้ใช้**：อ่าน/เขียน `/etc/crontab`, `/etc/cron.d/`, `/var/spool/cron/<user>` โดยตรง（แสดงอ่านอย่างเดียว + แก้ไขแบบควบคุม）

### 9.4 ผู้ช่วย AI

```
用户输入 → Service 拼上下文（当前页面/模块 + 系统状态）
  → 调 LLM（provider 适配：OpenAI/DeepSeek/Qwen/Ollama）
  → 若模型判定为「执行意图」→ 生成结构化 action + 参数
  → 命中权限白名单 → 执行 → 回传结果
  → 未授权/高危 → 要求用户二次确认
```

### 9.5 การเก็บเมตริก

- ตัวเก็บ：gopsutil สุ่มตัวอย่างทุก 5 วินาที → ring buffer ในหน่วยความจำ；
- ประวัติ：ลดความถี่แล้วเก็บ SQLite（ความละเอียด 1 นาที เก็บ 7 วัน）；
- การพุช：WebSocket ออกอากาศให้ผู้ติดตามแดชบอร์ด/หน้าตรวจสอบ

---

## 10. การตรวจสอบและการสังเกตการณ์

- บันทึกมีโครงสร้าง（zerolog）แบ่งระดับ ดูได้ในหน้า "บันทึก" ของแผง；
- เมตริก：เมตริกแผงเอง + โฮสต์เก็บรวมโดยโมดูล monitor；
- ตรวจสุขภาพ：`/api/healthz` คืนสถานะโปรเซส/DB/ดิสก์

---

## 11. การปรับใช้

### 11.1 ผลลัพธ์

- ไบนารีเดียว `opsmini`（ประมาณ 25~30MB หลังบีบอัด `-s -w`）มีหน้าเว็บ SQLite ทรัพยากรคงที่ในตัว；
- การตั้งค่า：`/etc/opsmini/config.yaml` หรือตัวแปรสภาพแวดล้อม；
- พอร์ตเริ่มต้น 8888 ไดเรกทอรีข้อมูล `/var/lib/opsmini/`（opsmini.db）

### 11.1.1 การเผยแพร่หลายสถาปัตยกรรม（x64 + arm64）

การเผยแพร่ต้องครอบคลุม **Linux** ทั้งชุดคำสั่ง **x86_64（amd64）** และ **ARM64（arm64）** ส่วนเป้าหมาย macOS คอมไพล์เพื่อการพัฒนาดีบักเท่านั้น

| แพลตฟอร์มเป้าหมาย | สถานการณ์ที่ใช้ |
|----------|----------|
| `linux/amd64` | เซิร์ฟเวอร์ x64 หลัก（Intel/AMD รุ่นทั่วไปของผู้ให้บริการคลาวด์） |
| `linux/arm64` | เซิร์ฟเวอร์ ARM（Graviton, Raspberry Pi, Kunpeng, Phytium เป็นต้น） |
| `darwin/arm64` | เครื่องพัฒนา Apple Silicon（ดีบักในเครื่อง） |
| `darwin/amd64` | Intel Mac（พัฒนาดีบัก） |

> OpsMini มุ่งเป้า**โฮสต์ Linux** เผยแพร่เฉพาะแพ็กเกจติดตั้ง Linux；เป้าหมาย macOS ใช้เพื่อการพัฒนาดีบักเท่านั้น ไม่ใช่ผลลัพธ์ที่ส่งมอบ

**ข้อกำหนดหลัก**：ชั้นข้อมูลใช้ไดรเวอร์ Pure Go `glebarez/sqlite`（ฐาน `modernc.org/sqlite`）**ไม่มีการพึ่งพา CGO** ดังนั้น `CGO_ENABLED=0` จึงคอมไพล์ข้ามแพลตฟอร์มไบนารี**ลิงก์แบบคงที่**ได้ด้วยคลิกเดียวบนแพลตฟอร์มใดก็ได้ ไม่ต้องเตรียม C cross toolchain ต่อสถาปัตยกรรม

**วิธีการบิลด์**（`Makefile` พร้อมแล้ว）：

```bash
make build        # 当前平台
make build-all    # 全平台交叉编译 → dist/
```

การตั้งชื่อผลลัพธ์：`opsmini-<version>-<os>-<arch>` หมายเลขเวอร์ชันฉีดผ่าน `-ldflags -X main.version` ตรวจสอบตอนรันด้วย `opsmini -version`

ผ่านการทดสอบจริง：ทั้ง 4 แพลตฟอร์มเป้าหมายคอมไพล์สำเร็จ `file` ตรวจสอบสถาปัตยกรรมถูกต้อง（ELF x86-64 / ELF aarch64 / Mach-O arm64 / Mach-O x86_64）และทั้งหมดลิงก์แบบคงที่

### 11.2 การทำเป็นบริการ

```ini
[Unit]
Description=OpsMini Agent
After=network.target

[Service]
ExecStart=/usr/local/bin/opsmini agent --config /etc/opsmini/config.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

### 11.3 reverse proxy（ทางเลือก）

Nginx reverse proxy 80/443 → 8888 ใบรับรองจัดการโดยโมดูลเว็บไซต์ในแผงหรือโหลดบาลานเซอร์ภายนอก

---

## 12. แผนการพัฒนา（ไมล์สโตน）

| ระยะ | เนื้อหา | การส่งมอบ |
|------|------|------|
| M1 โครงสร้าง | โปรเจกต์ Go, เราเตอร์ Gin, SQLite, การย้าย GORM, เข้าสู่ระบบ/ผู้ใช้/RBAC | แผงขั้นต่ำที่รันได้ |
| M2 ความสามารถระบบ | เมตริก gopsutil, โปรเซส/พอร์ต/ดิสก์/เครือข่าย, แดชบอร์ด+ตรวจสอบ | วงจรตรวจสอบเครื่องเดียว |
| M3 จัดการทรัพยากร | เว็บไซต์(Nginx), ฐานข้อมูล, ไฟล์, เทอร์มินัล(pty), งานตามกำหนดเวลา | เทียบเท่า 1Panel หลัก |
| M4 คอนเทนเนอร์ | Docker SDK：คอนเทนเนอร์/อิมเมจ/โวลุ่ม/เครือข่าย | จัดการคอนเทนเนอร์ |
| M5 AI | ชั้นอะแดปเตอร์ LLM, บริบท, การดำเนินการ NL, ผู้ช่วย AI | ความสามารถที่แตกต่าง |
| M6 Agent API | Agent เปิดเผย REST API มาตรฐาน（`/agent/v1`） การรับรอง token รายการอนุญาตคำสั่ง | ความสามารถบูรณาการภายนอก |
| M7 ขัดเกลา | i18n, การตรวจสอบ, การจำกัดอัตรา, การทดสอบ, เอกสาร | พร้อมใช้งานจริง |

---

## 13. ประเด็นการตัดสินใจที่ต้องยืนยัน

1. **ความเข้มข้นการรับรอง Agent API**：ค่าเริ่มต้น Bearer Token；ต้องใช้ mTLS ใบรับรองสองทางหรือไม่（ปลอดภัยกว่า แต่ปรับใช้หนักกว่า）？
2. **Agent API ต้องมีรายการอนุญาต IP หรือไม่**：ค่าเริ่มต้นปิด ใช้ Token เท่านั้น；สถานการณ์หลายเครื่อง/เครือข่ายสาธารณะเพิ่มการจำกัด IP หรือไม่？
3. **ข้อมูลสตรีม**：tail บันทึก / เมตริกเรียลไทม์ ค่าเริ่มต้นโพลแบบแบ่งหน้า REST ยอมรับได้หรือไม่？（v1.0 ไม่นำ SSE/การเชื่อมต่อยาว）
4. **การจัดการหลายเครื่องรวมศูนย์（อนาคต）**：หากนำ Proxy ศูนย์กลางมาใช้ภายหลัง จะใช้ซ้ำ `/agent/v1` ที่มีอยู่ทำการดึง/จัดตาราง หรือตั้งโปรโตคอลแยก？（นอกขอบเขต v1.0 บันทึกเท่านั้น）
