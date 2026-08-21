<div align="center">

# OpsMini Backend Architecture

**Backend-Architektur-Design**

[English](backend-architecture.md) · [简体中文](backend-architecture.zh-CN.md) · [繁體中文](backend-architecture.zh-TW.md) · [日本語](backend-architecture.ja.md) · [한국어](backend-architecture.ko.md) · [ไทย](backend-architecture.th.md) · [Deutsch](backend-architecture.de.md)

</div>

---

> Version: v1.0  
> Grundlage: `web/index.html` UI-Prototyp (Login / Benutzer / Dashboard / Monitoring / Apps / Container / System / Dateien / Terminal / Cron-Jobs / Logs / KI-Assistent / Panel-Einstellungen / i18n mit 7 Sprachen)

---

## 1. Überblick

OpsMini ist ein **leichtgewichtiges Single-Machine-Operations-Panel** (vergleichbar mit BaoTa / 1Panel) mit zwei wesentlichen Alleinstellungsmerkmalen:

1. **Eingebautes großes KI-Modell** für die Systemverwaltung (Diagnose / Ausführung / Log-Analyse in natürlicher Sprache);
2. **Stellt standardisierte REST-APIs nach außen bereit**, die von externen Systemen (Monitoring-Plattformen, Automatisierungsskripte, Orchestrierungswerkzeuge von Drittanbietern) integriert und aufgerufen werden können.

> Hinweis zum Umfang von v1.0: Diese Version enthält **keinen zentralen Proxy-Knoten**. Das OpsMini-Panel auf jedem Host arbeitet unabhängig
> und stellt seine Fähigkeiten über standardisierte REST-APIs nach außen bereit; die einheitliche Verwaltung mehrerer Maschinen (Proxy) wird in einer späteren Version evaluiert.

### 1.1 Designziele

| Ziel | Beschreibung |
|------|------|
| Leichtgewichtig | Bereitstellung als einzelnes Binärpaket, geringer Speicherverbrauch, geeignet für kleine 1C1G-Hosts |
| Single-Machine zuerst | Das Kernszenario ist ein einzelner Server; keine verteilte Komplexität |
| Integrierbar | Stellt Fähigkeiten über standardisierte REST-APIs zur Integration durch externe Systeme bereit |
| Sicher | RBAC, 2FA, sicherer Einstieg, minimale Rechte, verschlüsselte Schlüsselspeicherung |
| Wartbar | Modulare Schichtung mit klarer Controller → Service → Repository-Trennung |

### 1.2 Technologie-Stack

| Schicht | Auswahl | Alternative | Begründung |
|----|------|------|------|
| Sprache | Go 1.22+ | — | Einzelnes Binärpaket, Cross-Kompilierung, gute Nebenläufigkeit, ausgereiftes Ökosystem (gleicher Stack wie 1Panel) |
| Web-Framework | **Gin** | Echo / chi | Größtes Ökosystem, reichhaltige Middleware, identisch mit 1Panel |
| Datenbank | **SQLite** | — | Eingebettet, null Betriebsaufwand, passt zum Single-Machine-Szenario |
| SQLite-Treiber | **modernc.org/sqlite** | mattn/go-sqlite3 | Reines Go ohne CGO, einfache Cross-Kompilierung |
| ORM | **GORM** | sqlx | Hohe Entwicklungseffizienz; komplexe Abfragen können auf natives SQL zurückfallen |
| Echtzeitkommunikation | **gorilla/websocket** | — | Terminal, Log-tail, Metrik-Push |
| Aufgabenplanung | **robfig/cron** | — | Panel-Cron-Jobs |
| System-Monitoring | **gopsutil** | /proc lesen | Plattformübergreifend CPU/Speicher/Disk/Prozesse |
| Docker | Offizielles SDK | — | Container/Images/Volumes/Netzwerke |
| Logging | zerolog | zap | Leichtgewichtig, strukturiert, geringe Allokation |
| Authentifizierung | JWT + refresh | session | Zustandslose API + optionale Sitzung |
| 2FA | TOTP (RFC 6238) | — | Wiederverwendung von pquerna/otp |
| KI | Abstrakte LLM-Schnittstelle | — | Einheitliche Adapter für OpenAI/DeepSeek/Qwen/Ollama |

### 1.3 Gesamtarchitektur

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

## 2. Kernentscheidungen der Architektur

### 2.1 Monolith als einzelnes Binärpaket, keine Microservices

**Entscheidung**: Monolith mit einem Prozess. Das Frontend-Build-Artefakt (Vue3) wird über `go:embed` in das Binärpaket eingebettet; am Ende wird eine einzige ausführbare Datei `opsmini` ausgeliefert.

**Begründung**:
- Der Kernbedarf eines Operations-Panels ist „auf einer Maschine installieren, diese Maschine verwalten"; ein Monolith passt am besten;
- Die externe Integration erfolgt über standardisierte REST-APIs und steht nicht im Widerspruch zum Monolithen;
- Microservices bringen unnötige Komplexität wie Deployment, Service-Discovery und verteilte Transaktionen mit sich.

### 2.2 Frontend-/Backend-Trennung + eingebettetes Packaging

- Entwicklung: Der Vue3-Dev-Server proxyt auf Gin (CORS / Reverse-Proxy);
- Produktion: Das Artefakt `web/dist` wird über `go:embed` in das Binärpaket eingebettet, Bereitstellung als einzelne Datei.

### 2.3 SQLite statt MySQL/Postgres

- Das Geschäftsdatenvolumen ist klein (Konfiguration, Jobs, Benutzer, Logs); SQLite ist völlig ausreichend;
- Null Betriebsaufwand, einzelne Datei, einfaches Backup (.db direkt kopieren);
- Falls die Panel-eigenen Daten später hohe Parallelität benötigen, kann über eine Repository-Schnittstellenabstraktion gewechselt werden.

### 2.4 Bereitstellung standardisierter REST-APIs nach außen

- v1.0 **entwickelt keinen zentralen Proxy-Knoten**; es werden nur die Panel-Fähigkeiten als standardisierte REST-APIs bereitgestellt;
- Zwei APIs existieren parallel: `/api/v1` (für die Browser-UI) und `/agent/v1` (für Maschinen/Integration von Drittanbietern);
- Die Agent-API verwendet eine unabhängige Authentifizierung (Agent Token) + Befehls-Allowlist, getrennt von den Benutzer-Sitzungs-JWTs;
- Falls später eine einheitliche Verwaltung mehrerer Maschinen nötig ist, kann eine Proxy-Pull-/Scheduling-Schicht über der Agent-API ergänzt werden (außerhalb des v1.0-Umfangs).

---

## 3. Modulaufteilung (entsprechend dem UI-Prototyp)

| Backend-Modul | Zuständigkeit | Zugehörige Prototyp-Seite |
|----------|------|-------------|
| `auth` | Login/Logout, Sitzungen, 2FA, RBAC | Login-Seite, Benutzerverwaltung |
| `setting` | Panel-Konfiguration, Theme, Menü-Sichtbarkeit, Sprache | Panel-Einstellungen (Basis/Erscheinungsbild/Menü) |
| `dashboard` | Metrik-Aggregation, Echtzeit-Push | Dashboard |
| `monitor` | Zeitreihen-Erfassung, Alarmregeln, Alarmauslösung | Monitoring |
| `website` | Nginx-Sites, Domains, SSL-Zertifikate | App-Verwaltung-Websites |
| `database` | MySQL/PostgreSQL-Instanzen und -Datenbanken | App-Verwaltung-Datenbanken |
| `store` | Software-Installation/Deinstallation/Upgrade | App-Verwaltung-Software-Store |
| `container` | Docker-Container/Images/Volumes/Netzwerke | Container-Verwaltung |
| `system` | Prozesse/Netzwerk/Ports/Datenträger | Systemverwaltung |
| `file` | Datei-Durchsuchen/Upload/Bearbeitung/Berechtigungen | Dateien |
| `terminal` | Web-SSH | Terminal |
| `cron` | Cron-Jobs (Panel + System + Benutzer-crontab) | Cron-Jobs |
| `log` | Log-Sammlung/Aggregation/tail | Logs |
| `ai` | LLM-Anbindung, Kontext, NL-Ausführung | KI-Assistent, Panel-Einstellungen-KI |
| `agent` | Externe REST-API (`/agent/v1`), Agent-Token-Authentifizierung, Befehls-Allowlist | Panel-Einstellungen-Proxy-Anbindung |

---

## 4. Schichtung und Verzeichnisstruktur

### 4.1 Schichtung

Jedes Modul folgt intern strikt drei Schichten:

```
Controller（HTTP handler）：参数解析校验、调用 Service、组装响应
    → Service（业务逻辑）：编排 Repository 与系统资源、事务边界
    → Repository（GORM）：纯数据访问，不写业务
```

### 4.2 Verzeichnisstruktur

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

## 5. Datenmodell (SQLite-Schema)

> GORM-Migration; sensible Felder (Schlüssel/Token) werden vor der Speicherung mit AES-GCM verschlüsselt.

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

## 6. API-Design (RESTful, `/api/v1`)

Einheitliche Antwort: `{ "code": 0, "message": "ok", "data": ... }`; ein `code` ungleich 0 bedeutet einen Geschäftsfehler.

| Methode | Pfad | Beschreibung |
|------|------|------|
| POST | `/auth/login` | Login (gibt access + refresh zurück) |
| POST | `/auth/refresh` | Token aktualisieren |
| POST | `/auth/logout` | Abmelden |
| GET | `/auth/2fa/qrcode` | 2FA-QR-Code generieren |
| POST | `/auth/2fa/verify` | 2FA verifizieren |
| GET | `/users` / POST / PUT / DELETE | Benutzer-CRUD |
| GET | `/dashboard/metrics` | Dashboard-Metriken |
| GET | `/monitor/series?range=1h` | Zeitreihendaten |
| CRUD | `/alert-rules` | Alarmregeln |
| CRUD | `/websites` | Websites |
| POST | `/websites/:id/ssl` | SSL ausstellen/verlängern |
| CRUD | `/databases` | Datenbanken |
| GET | `/store/apps` / POST `/store/apps/:id/install` | Software-Store |
| GET | `/containers` / `/images` / `/volumes` / `/networks` | Die vier Container-Ressourcen |
| POST | `/containers` usw. | Container erstellen/Image pullen/Volume erstellen/Netzwerk erstellen |
| GET | `/system/processes` `/networks` `/ports` `/disks` | Systemressourcen |
| GET/POST | `/files` / `/files/list` / `/files/upload` / `/files/edit` | Dateien |
| WS  | `/terminal/ws?cols=&rows=` | Web-SSH |
| CRUD | `/cron-jobs` | Cron-Jobs (Panel-Typ) |
| GET | `/cron-jobs/system` `/cron-jobs/user` | System-/Benutzer-crontab (schreibgeschützt) |
| GET | `/logs` | Log-Liste |
| POST | `/ai/chat` | KI-Chat (Streaming SSE) |
| POST | `/ai/execute` | NL-zu-Aktion (mit Berechtigungsbestätigung) |
| GET/PUT | `/agent/config` | Agent-API-Konfiguration (Token, Befehls-Allowlist) |
| GET/PUT | `/settings` | Panel-Einstellungen |
| GET | `/i18n/{lang}` | Sprachpaket (Frontend kann es auch inline einbetten) |
| GET | `/metrics` | **Prometheus-Metriken** (node_exporter-kompatibel, ohne `/api/v1`-Präfix) |

### 6.1 Prometheus-Monitoring-Integration

OpsMini besitzt einen eingebauten **node_exporter-kompatiblen `/metrics`-Endpunkt**; ein separater node_exporter ist nicht nötig, Prometheus kann direkt scrapen:

```
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
```

Die Kernmetriken von node_exporter sind abgestimmt (das Community Node Dashboard kann direkt verwendet werden):

| Metrik-Familie | Beschreibung |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | Kumulierte Sekunden pro Kern und Modus |
| `node_memory_MemTotal_bytes` usw. | Speicher Total/Free/Available/Buffers/Cached |
| `node_filesystem_size_bytes{mountpoint}` | Kapazität/Verfügbar/Nutzung pro Mountpoint |
| `node_network_receive_bytes_total{device}` | RX/TX-Verkehr pro Netzwerkkarte |
| `node_load1` / `node_load5` / `node_load15` | Last |
| `node_uname_info` / `node_boot_time_seconds` | Host-Informationen und Startzeit |

> Umsetzung: Wiederverwendung von `gopsutil` (bereits vom SystemService erfasste Daten), Ausgabe im Prometheus-Textformat,
> Beibehaltung der Auslieferung als einzelnes Binärpaket, ohne Einbettung eines node_exporter-Prozesses.

### 6.2 Agent-API (externe standardisierte REST-Schnittstelle)

Neben der von der UI genutzten `/api/v1` stellt das Panel eine **unabhängige Maschine-zu-Maschine-REST-API** (`/agent/v1`) bereit, die externe Systeme (Monitoring-Plattformen, Automatisierungsskripte, Orchestrierungswerkzeuge von Drittanbietern) integrieren und aufrufen können. Unterschiede zur Panel-API:

- **Authentifizierung**: Verwendung von Agent Token statt Benutzer-Sitzungs-JWT;
- **Umfang**: Fokussiert auf Ressourcenverwaltung und Befehlsausführung, ohne UI-spezifische Fähigkeiten (i18n/Theme/Menü) und Terminal-WS;
- **Stil**: Automatisierungsorientiert — idempotent, wiederholbar, einheitliche JSON-Antworten.

| Methode | Pfad | Beschreibung |
|------|------|------|
| GET | `/agent/v1/health` | Health-Check (Liveness) |
| GET | `/agent/v1/status` | Statusübersicht: cpu/mem/Disk/Online-Dienste |
| GET | `/agent/v1/system/info` | Host-Informationen (hostname/os/Kernel) |
| GET | `/agent/v1/system/processes` | Prozessliste |
| GET | `/agent/v1/system/ports` | Lauschende Ports |
| GET | `/agent/v1/system/disks` | Datenträger/Mountpoints |
| GET | `/agent/v1/websites` / POST | Website-Abfrage/Erstellung |
| GET | `/agent/v1/databases` | Datenbankliste |
| GET | `/agent/v1/containers` | Containerliste |
| GET | `/agent/v1/cron-jobs` | Cron-Jobs |
| POST | `/agent/v1/commands` | Befehl ausführen (Allowlist, gibt Ausführungsergebnis zurück) |

---

## 7. Design der externen REST-API (Differenzierungsschwerpunkt)

### 7.1 Positionierung

OpsMini v1.0 ist ein **Single-Machine-Panel**, das Fähigkeiten über standardisierte REST-APIs nach außen bereitstellt, damit externe Systeme sie integrieren können:

- **`/api/v1` (Panel-API)**: für die Browser-UI, Benutzer-Sitzungs-JWT + RBAC;
- **`/agent/v1` (Agent-API)**: für Maschinen/Integration von Drittanbietern, Agent-Token-Authentifizierung + Befehls-Allowlist.

> Anders als der „ausgehende Long-Lived-Connection-Push" von salt minion/master stellt OpsMini **REST direkt bereit, damit externe Systeme per Pull aufrufen** —
> näher am Ansatz „Prometheus pullt exporter / Cloud-Anbieter-OpenAPI". Ob ein Proxy-Zentrum für die einheitliche Verwaltung mehrerer Maschinen eingeführt wird, wird in einer späteren Version evaluiert (nicht in v1.0).

### 7.2 Aufrufmodell

```
         HTTPS REST 调用（Agent Token）
   ┌──────────┐  ─────────────────────▶  ┌─────────┐
   │ 外部系统  │                          │ OpsMini │
   │ (监控/脚本 │  ◀─────────────────────  │ (单机面板)│
   │ /编排工具) │       统一 JSON 响应      └─────────┘
   └──────────┘
```

- **Keine Long-Lived-Connections**: Alles läuft über Standard-REST, ohne WebSocket-/gRPC-Long-Lived-Connections;
- **Streaming-Szenarien** (Log-tail, Echtzeit-Metriken): REST-Paginierungs-Polling genügt, kein SSE nötig;
- **Idempotenz**: Lesende GETs sind von Natur aus idempotent; Schreiboperationen (Befehlsausführung, Ressourcenerstellung) liefern eindeutige Ergebnisse.

### 7.3 Schlüsselabläufe

1. Panel startet → liest `agent_config` (Token + Befehls-Allowlist);
2. Externes System ruft `/agent/v1/*` mit `Authorization: Bearer <token>` auf;
3. Middleware prüft das Token (Vergleich in konstanter Zeit) → bei Allowlist-Treffer Freigabe, sonst 401;
4. Hochriskante Operationen (Befehlsausführung) durchlaufen eine zweite Befehls-Allowlist-Prüfung; alles wird auditiert;
5. Gibt einheitliches JSON zurück: `{ code, message, data }`.

### 7.4 Sicherheit

- **Authentifizierung**: Das Panel konfiguriert ein unabhängiges zufälliges Agent-Token (verschlüsselt gespeichert), Request-Header `Authorization: Bearer <token>`;
- **Transport**: HTTPS; in Szenarien mit hoher Sicherheit kann eine IP-Allowlist ergänzt werden;
- **Befehls-Allowlist**: `/commands` erlaubt nur explizit autorisierte Befehle; alles wird auditiert;
- **Minimale Exposition**: `/agent/v1` ist von `/api/v1` getrennt; die Agent-API stellt keine UI-Fähigkeiten und kein Terminal bereit.

### 7.5 Positionierung der beiden APIs

| Dimension | Panel-API `/api/v1` | Agent-API `/agent/v1` |
|------|-------------------|----------------------|
| Nutzer | Browser (Mensch) | Externes System (Maschine) |
| Authentifizierung | Benutzer-Sitzungs-JWT + RBAC | Agent Token |
| Umfang | Alle UI-Funktionen + Terminal-WS | Ressourcenverwaltung + Befehlsausführung (ohne UI/Terminal) |
| Design | Interaktionsorientiert | Automatisierungsorientiert (idempotent, wiederholbar) |

---

## 8. Sicherheitsdesign

| Dimension | Ansatz |
|------|------|
| Passwort | bcrypt-Hash |
| Sitzung | Kurzlebiges Access-JWT (15 min) + Refresh-Token (widerrufbar) |
| 2FA | TOTP (RFC 6238), optionaler zweiter Faktor beim Login |
| Autorisierung | RBAC mit drei Rollen: admin (alles) / operator (täglicher Betrieb) / readonly (schreibgeschützt) |
| Sicherer Einstieg | Panel-Zugriff erfordert einen geheimen Pfad (z. B. `/opsmini_panel`) gegen Port-Scans |
| Schlüsselspeicherung | Panel-Schlüssel (API Key, Agent Token) AES-GCM-verschlüsselt gespeichert |
| Terminal | Web-SSH nur für operator und höher, Sitzungen werden aufgezeichnet und auditiert |
| Schutz vor Brute-Force | Ratenbegrenzung bei Login-Fehlern + Sperre |
| Audit | Kritische Operationen werden in `audit_logs` geschrieben |
| CSRF/XSS | API nutzt Bearer Token ohne Cookies; Frontend escapt Ausgabe |

---

## 9. Schlüsselabläufe

### 9.1 Login

```
输入账密 → bcrypt 校验 → 若开启 2FA 则要求 TOTP
  → 签发 access + refresh → 前端存 refresh（httpOnly/localStorage）
  → 后续请求带 Bearer access → 过期用 refresh 换新
```

### 9.2 Web-SSH-Terminal

```
前端 ws://host/api/v1/terminal/ws?token=...
  → 服务端校验 token + 角色
  → 启动 pty（github.com/creack/pty）→ 双向数据转发
  → 关闭时回收 pty、记录会话时长
```

### 9.3 Ausführung von Cron-Jobs

- **Panel-Jobs**: robfig/cron als residenter Scheduler, schreibt in `cron_jobs`;
- **System-/Benutzer-Jobs**: Direktes Lesen/Schreiben von `/etc/crontab`, `/etc/cron.d/`, `/var/spool/cron/<user>` (schreibgeschützte Anzeige + kontrollierte Bearbeitung).

### 9.4 KI-Assistent

```
用户输入 → Service 拼上下文（当前页面/模块 + 系统状态）
  → 调 LLM（provider 适配：OpenAI/DeepSeek/Qwen/Ollama）
  → 若模型判定为「执行意图」→ 生成结构化 action + 参数
  → 命中权限白名单 → 执行 → 回传结果
  → 未授权/高危 → 要求用户二次确认
```

### 9.5 Metrik-Erfassung

- Collector: gopsutil sampelt alle 5 s → Ring-Puffer im Speicher;
- Historie: nach Downsampling in SQLite (1-Minuten-Granularität, 7 Tage Aufbewahrung);
- Push: WebSocket-Broadcast an Abonnenten der Dashboard-/Monitoring-Seite.

---

## 10. Monitoring und Observability

- Strukturierte Logs (zerolog), gestufte Level, einsehbar auf der „Logs"-Seite des Panels;
- Metriken: Panel-eigene + Host-Metriken werden einheitlich vom monitor-Modul erfasst;
- Health-Check: `/api/healthz` gibt Prozess-/DB-/Disk-Status zurück.

---

## 11. Deployment

### 11.1 Artefakte

- `opsmini` als einzelnes Binärpaket (ca. 25~30 MB nach `-s -w`-Kompression), mit eingebettetem Frontend, SQLite und statischen Ressourcen;
- Konfiguration: `/etc/opsmini/config.yaml` oder Umgebungsvariablen;
- Standardport 8888, Datenverzeichnis `/var/lib/opsmini/` (opsmini.db).

### 11.1.1 Multi-Architektur-Releases (x64 + arm64)

Releases müssen **Linux** mit den beiden Befehlssätzen **x86_64 (amd64)** und **ARM64 (arm64)** abdecken; ein macOS-Target wird nur für Entwicklung/Debugging kompiliert.

| Zielplattform | Einsatzszenario |
|----------|----------|
| `linux/amd64` | Gängige x64-Server (Intel/AMD, übliche Cloud-Anbieter-Modelle) |
| `linux/arm64` | ARM-Server (Graviton, Raspberry Pi, Kunpeng, Phytium usw.) |
| `darwin/arm64` | Apple-Silicon-Entwicklungsrechner (lokales Debugging) |
| `darwin/amd64` | Intel Mac (Entwicklung/Debugging) |

> OpsMini zielt auf **Linux-Hosts**; es werden nur Linux-Installationspakete veröffentlicht. Das macOS-Target dient nur Entwicklung/Debugging und ist kein Auslieferungsartefakt.

**Zentrale Voraussetzung**: Die Datenschicht verwendet den reinen Go-Treiber `glebarez/sqlite` (Basis `modernc.org/sqlite`) **ohne CGO-Abhängigkeit**. Daher ermöglicht `CGO_ENABLED=0` die Cross-Kompilierung eines **statisch gelinkten** Binärpakets auf jeder Plattform per Ein-Klick, ohne eine C-Cross-Toolchain pro Architektur vorzubereiten.

**Build-Methode** (`Makefile` bereits vorhanden):

```bash
make build        # 当前平台
make build-all    # 全平台交叉编译 → dist/
```

Artefakt-Benennung: `opsmini-<version>-<os>-<arch>`, Versionsnummer wird über `-ldflags -X main.version` injiziert und zur Laufzeit per `opsmini -version` abfragbar.

Praktisch verifiziert: Alle 4 Zielplattformen kompilieren erfolgreich; `file` bestätigt die korrekte Architektur (ELF x86-64 / ELF aarch64 / Mach-O arm64 / Mach-O x86_64) und alle sind statisch gelinkt.

### 11.2 Als Dienst installieren

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

### 11.3 Reverse-Proxy (optional)

Nginx als Reverse-Proxy 80/443 → 8888; Zertifikate werden vom Website-Modul im Panel oder einem externen Load-Balancer verwaltet.

---

## 12. Entwicklungs-Roadmap (Meilensteine)

| Phase | Inhalt | Lieferung |
|------|------|------|
| M1 Grundgerüst | Go-Projekt, Gin-Routing, SQLite, GORM-Migration, Login/Benutzer/RBAC | Minimal lauffähiges Panel |
| M2 Systemfähigkeiten | gopsutil-Metriken, Prozesse/Ports/Datenträger/Netzwerk, Dashboard + Monitoring | Geschlossener Single-Machine-Monitoring-Kreislauf |
| M3 Ressourcenverwaltung | Websites (Nginx), Datenbanken, Dateien, Terminal (pty), Cron-Jobs | Parität mit 1Panel-Kern |
| M4 Container | Docker SDK: Container/Images/Volumes/Netzwerke | Container-Verwaltung |
| M5 KI | LLM-Adapter-Schicht, Kontext, NL-Ausführung, KI-Assistent | Differenzierungsfähigkeit |
| M6 Agent-API | Agent stellt standardisierte REST-API (`/agent/v1`) bereit, Token-Auth, Befehls-Allowlist | Externe Integrationsfähigkeit |
| M7 Feinschliff | i18n, Audit, Ratenbegrenzung, Tests, Dokumentation | Produktionsreif |

---

## 13. Zu bestätigende Entscheidungspunkte

1. **Authentifizierungsstärke der Agent-API**: Standard Bearer Token; sind mTLS-Zertifikate nötig (sicherer, aber schwereres Deployment)?
2. **Benötigt die Agent-API eine IP-Allowlist**: standardmäßig deaktiviert, nur Token; sollen IP-Beschränkungen für Multi-Machine-/Public-Network-Szenarien ergänzt werden?
3. **Streaming-Daten**: Log-tail / Echtzeit-Metriken standardmäßig über REST-Paginierungs-Polling — akzeptabel? (v1.0 führt kein SSE/Long-Lived-Connection ein)
4. **Einheitliche Multi-Machine-Verwaltung (Zukunft)**: Falls später ein Proxy-Zentrum eingeführt wird — die bestehende `/agent/v1` für Pull/Scheduling wiederverwenden oder ein separates Protokoll definieren? (außerhalb des v1.0-Umfangs, nur Notiz)
