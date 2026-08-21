<div align="center">

# OpsMini Backend Architecture

**백엔드 아키텍처 설계**

[English](backend-architecture.md) · [简体中文](backend-architecture.zh-CN.md) · [繁體中文](backend-architecture.zh-TW.md) · [日本語](backend-architecture.ja.md) · [한국어](backend-architecture.ko.md) · [ไทย](backend-architecture.th.md) · [Deutsch](backend-architecture.de.md)

</div>

---

> 버전：v1.0  
> 근거：`web/index.html` UI 프로토타입（로그인/사용자/대시보드/모니터링/앱/컨테이너/시스템/파일/터미널/크론 작업/로그/AI 어시스턴트/패널 설정/7개 언어 i18n）

---

## 1. 개요

OpsMini는 **경량 단일 머신 운영 패널**(보타 / 1Panel 대응)로, 두 가지 차별점이 있습니다：

1. **내장 AI 대형 모델**로 시스템 관리(자연어 진단 / 실행 / 로그 분석)；
2. **표준 REST API를 외부에 노출**하여 외부 시스템(모니터링 플랫폼, 자동화 스크립트, 서드파티 오케스트레이션 도구)에서 통합 호출할 수 있습니다。

> v1.0 범위 설명：이 버전에는 **Proxy 중앙 노드가 포함되지 않습니다**. 각 호스트의 OpsMini 패널은 독립적으로 동작하며,
> 표준 REST API를 통해 외부에 기능을 제공합니다. 다중 머신 통합 관리(Proxy)는 후속 버전에서 평가합니다。

### 1.1 설계 목표

| 목표 | 설명 |
|------|------|
| 경량 | 단일 바이너리 배포, 낮은 메모리 점유, 1C1G 소형 호스트에 적합 |
| 단일 머신 우선 | 핵심 시나리오는 단일 서버이며, 분산 복잡성을 도입하지 않음 |
| 통합 가능 | 표준 REST API로 기능을 노출하여 외부 시스템이 통합 호출 가능 |
| 보안 | RBAC, 2FA, 보안 진입점, 최소 권한, 키 암호화 저장 |
| 유지보수성 | 모듈형 계층화, Controller → Service → Repository 명확 |

### 1.2 기술 스택 선정

| 계층 | 선정 | 대안 | 이유 |
|----|------|------|------|
| 언어 | Go 1.22+ | — | 단일 바이너리, 크로스 컴파일, 뛰어난 동시성, 성숙한 생태계(1Panel과 동일 스택) |
| 웹 프레임워크 | **Gin** | Echo / chi | 최대 생태계, 풍부한 미들웨어, 1Panel과 동일 |
| 데이터베이스 | **SQLite** | — | 임베디드, 제로 운영, 단일 머신 시나리오에 적합 |
| SQLite 드라이버 | **modernc.org/sqlite** | mattn/go-sqlite3 | 순수 Go로 CGO 불필요, 크로스 컴파일 간편 |
| ORM | **GORM** | sqlx | 개발 효율 높음. 복잡한 쿼리는 네이티브 SQL로 폴백 가능 |
| 실시간 통신 | **gorilla/websocket** | — | 터미널, 로그 tail, 지표 푸시 |
| 작업 스케줄링 | **robfig/cron** | — | 패널 크론 작업 |
| 시스템 모니터링 | **gopsutil** | /proc 읽기 | 크로스 플랫폼 CPU/메모리/디스크/프로세스 |
| Docker | 공식 SDK | — | 컨테이너/이미지/볼륨/네트워크 |
| 로그 | zerolog | zap | 경량, 구조화, 낮은 할당 |
| 인증 | JWT + refresh | session | 무상태 API + 선택적 세션 |
| 2FA | TOTP（RFC 6238） | — | pquerna/otp 재사용 |
| AI | 추상 LLM 인터페이스 | — | OpenAI/DeepSeek/Qwen/Ollama 통합 어댑터 |

### 1.3 전체 아키텍처

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

## 2. 핵심 아키텍처 결정

### 2.1 모놀리스 단일 바이너리, 마이크로서비스 미도입

**결정**：단일 프로세스 모놀리스. 프런트엔드 Vue3 빌드 산출물을 `go:embed`로 바이너리에 임베드하고, 최종적으로 단일 `opsmini` 실행 파일을 제공합니다.

**이유**：
- 운영 패널의 핵심 요구는 "한 대 설치하고 한 대 관리"이며, 모놀리스가 가장 적합합니다；
- 외부 통합은 표준 REST API로 수행되므로 모놀리스와 충돌하지 않습니다；
- 마이크로서비스는 배포, 서비스 디스커버리, 분산 트랜잭션 등 불필요한 복잡성을 초래합니다。

### 2.2 프런트엔드/백엔드 분리 + 임베드 패키징

- 개발기：Vue3 dev server가 Gin으로 프록시(CORS / 리버스 프록시)；
- 운영기：`web/dist` 산출물을 `go:embed`로 바이너리에 임베드하여 단일 파일 배포。

### 2.3 MySQL/Postgres 대신 SQLite

- 비즈니스 데이터 양이 적어(설정, 작업, 사용자, 로그) SQLite로 충분합니다；
- 제로 운영, 단일 파일, 쉬운 백업(.db 직접 복사)；
- 향후 패널 자체 데이터에 높은 동시성이 필요하면 Repository 인터페이스를 추상화하여 전환 가능。

### 2.4 표준 REST API 외부 노출

- v1.0은 **Proxy 중앙 노드를 개발하지 않고**, 패널 기능만 표준 REST API로 노출합니다；
- 두 API 공존：`/api/v1`(브라우저 UI용)과 `/agent/v1`(머신/서드파티 통합용)；
- Agent API는 독립 인증(Agent Token) + 명령 허용 목록으로, 사용자 세션 JWT와 분리；
- 향후 다중 머신 통합 관리가 필요하면 Agent API 위에 Proxy 풀/스케줄링 계층을 추가 가능(v1.0 범위 밖)。

---

## 3. 모듈 분할（UI 프로토타입 대응）

| 백엔드 모듈 | 책임 | 대응 프로토타입 페이지 |
|----------|------|-------------|
| `auth` | 로그인/로그아웃, 세션, 2FA, RBAC | 로그인 페이지, 사용자 관리 |
| `setting` | 패널 설정, 테마, 메뉴 표시/숨김, 언어 | 패널 설정（기본/외관/메뉴） |
| `dashboard` | 지표 집계, 실시간 푸시 | 대시보드 |
| `monitor` | 시계열 수집, 알람 규칙, 알람 트리거 | 모니터링 |
| `website` | Nginx 사이트, 도메인, SSL 인증서 | 앱 관리-웹사이트 |
| `database` | MySQL/PostgreSQL 인스턴스와 DB | 앱 관리-데이터베이스 |
| `store` | 소프트웨어 설치/제거/업그레이드 | 앱 관리-소프트웨어 스토어 |
| `container` | Docker 컨테이너/이미지/볼륨/네트워크 | 컨테이너 관리 |
| `system` | 프로세스/네트워크/포트/디스크 | 시스템 관리 |
| `file` | 파일 탐색/업로드/편집/권한 | 파일 |
| `terminal` | Web SSH | 터미널 |
| `cron` | 크론 작업（패널+시스템+사용자 crontab） | 크론 작업 |
| `log` | 로그 수집/집계/tail | 로그 |
| `ai` | LLM 연결, 컨텍스트, NL 실행 | AI 어시스턴트, 패널 설정-AI |
| `agent` | 외부 REST API（`/agent/v1`）, Agent Token 인증, 명령 허용 목록 | 패널 설정-Proxy 연결 |

---

## 4. 계층화와 디렉터리 구조

### 4.1 계층화

각 모듈 내부는 엄격한 3계층：

```
Controller（HTTP handler）：参数解析校验、调用 Service、组装响应
    → Service（业务逻辑）：编排 Repository 与系统资源、事务边界
    → Repository（GORM）：纯数据访问，不写业务
```

### 4.2 디렉터리 구조

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

## 5. 데이터 모델（SQLite 스키마）

> GORM 마이그레이션. 민감 필드(키/Token)는 AES-GCM으로 암호화 후 저장.

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

## 6. API 설계（RESTful, `/api/v1`）

통합 응답：`{ "code": 0, "message": "ok", "data": ... }`, `code`가 0이 아니면 비즈니스 오류.

| 메서드 | 경로 | 설명 |
|------|------|------|
| POST | `/auth/login` | 로그인(access + refresh 반환) |
| POST | `/auth/refresh` | 토큰 갱신 |
| POST | `/auth/logout` | 로그아웃 |
| GET | `/auth/2fa/qrcode` | 2FA QR 코드 생성 |
| POST | `/auth/2fa/verify` | 2FA 검증 |
| GET | `/users` / POST / PUT / DELETE | 사용자 CRUD |
| GET | `/dashboard/metrics` | 대시보드 지표 |
| GET | `/monitor/series?range=1h` | 시계열 데이터 |
| CRUD | `/alert-rules` | 알람 규칙 |
| CRUD | `/websites` | 웹사이트 |
| POST | `/websites/:id/ssl` | SSL 발급/갱신 |
| CRUD | `/databases` | 데이터베이스 |
| GET | `/store/apps` / POST `/store/apps/:id/install` | 소프트웨어 스토어 |
| GET | `/containers` / `/images` / `/volumes` / `/networks` | 컨테이너 4종 |
| POST | `/containers` 등 | 컨테이너 생성/이미지 풀/볼륨 생성/네트워크 생성 |
| GET | `/system/processes` `/networks` `/ports` `/disks` | 시스템 리소스 |
| GET/POST | `/files` / `/files/list` / `/files/upload` / `/files/edit` | 파일 |
| WS  | `/terminal/ws?cols=&rows=` | Web SSH |
| CRUD | `/cron-jobs` | 크론 작업（패널 유형） |
| GET | `/cron-jobs/system` `/cron-jobs/user` | 시스템/사용자 crontab 읽기 전용 |
| GET | `/logs` | 로그 목록 |
| POST | `/ai/chat` | AI 대화（스트리밍 SSE） |
| POST | `/ai/execute` | NL을 작업으로（권한 확인 포함） |
| GET/PUT | `/agent/config` | Agent API 설정（token, 명령 허용 목록） |
| GET/PUT | `/settings` | 패널 설정 |
| GET | `/i18n/{lang}` | 언어 팩（프런트엔드에서도 인라인 가능） |
| GET | `/metrics` | **Prometheus 지표**（node_exporter 호환, `/api/v1` 접두사 없음） |

### 6.1 Prometheus 모니터링 통합

OpsMini는 **node_exporter 호환 `/metrics` 엔드포인트**를 내장하여 node_exporter를 별도 설치할 필요 없이 Prometheus가 직접 스크랩할 수 있습니다：

```
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
```

node_exporter의 핵심 지표에 정렬됨（커뮤니티 Node Dashboard를 그대로 적용 가능）：

| 지표 패밀리 | 설명 |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | 코어별 모드별 누적 초 |
| `node_memory_MemTotal_bytes` 등 | 메모리 Total/Free/Available/Buffers/Cached |
| `node_filesystem_size_bytes{mountpoint}` | 각 마운트 포인트 용량/가용/사용률 |
| `node_network_receive_bytes_total{device}` | 각 NIC 송수신 트래픽 |
| `node_load1` / `node_load5` / `node_load15` | 로드 |
| `node_uname_info` / `node_boot_time_seconds` | 호스트 정보와 부팅 시간 |

> 구현 방식：`gopsutil`(SystemService가 이미 수집한 데이터)을 재사용하여 Prometheus 텍스트 형식으로 출력,
> 단일 바이너리 제공을 유지하고 node_exporter 프로세스를 임베드하지 않음.

### 6.2 Agent API（외부용 표준 REST 인터페이스）

패널은 UI가 사용하는 `/api/v1` 외에도 **독립적인 머신 간 REST API**(`/agent/v1`)를 노출하여 외부 시스템(모니터링 플랫폼, 자동화 스크립트, 서드파티 오케스트레이션 도구)이 통합 호출할 수 있습니다. 패널 API와의 차이：

- **인증**：사용자 세션 JWT가 아닌 Agent Token 사용；
- **범위**：리소스 관리와 명령 실행에 집중하며 UI 전용 기능(i18n/테마/메뉴)과 터미널 WS는 포함하지 않음；
- **스타일**：자동화 지향——멱등, 재시도 가능, 통합 JSON 응답。

| 메서드 | 경로 | 설명 |
|------|------|------|
| GET | `/agent/v1/health` | 헬스 체크（활성 확인） |
| GET | `/agent/v1/status` | 상태 요약：cpu/mem/디스크/온라인 서비스 |
| GET | `/agent/v1/system/info` | 호스트 정보（hostname/os/커널） |
| GET | `/agent/v1/system/processes` | 프로세스 목록 |
| GET | `/agent/v1/system/ports` | 포트 리스닝 |
| GET | `/agent/v1/system/disks` | 디스크/마운트 포인트 |
| GET | `/agent/v1/websites` / POST | 웹사이트 조회/생성 |
| GET | `/agent/v1/databases` | 데이터베이스 목록 |
| GET | `/agent/v1/containers` | 컨테이너 목록 |
| GET | `/agent/v1/cron-jobs` | 크론 작업 |
| POST | `/agent/v1/commands` | 명령 실행（허용 목록, 실행 결과 반환） |

---

## 7. 외부 REST API 설계（차별화 중점）

### 7.1 포지셔닝

v1.0의 OpsMini는 **단일 머신 패널**로, 표준 REST API를 통해 외부에 기능을 노출하여 외부 시스템 통합에 제공합니다：

- **`/api/v1`（패널 API）**：브라우저 UI용, 사용자 세션 JWT + RBAC；
- **`/agent/v1`（Agent API）**：머신/서드파티 통합용, Agent Token 인증 + 명령 허용 목록。

> salt minion/master의 "아웃바운드 장기 연결 push"와 달리 OpsMini는 **REST를 노출하여 외부가 풀 호출**하도록 합니다.
> Prometheus가 exporter를 풀하는 방식이나 클라우드 벤더 OpenAPI의 사고에 가깝습니다. Proxy 센터를 도입하여 다중 머신을 통합 관리할지는 후속 버전에서 평가합니다(v1.0에서는 하지 않음).

### 7.2 호출 모델

```
         HTTPS REST 调用（Agent Token）
   ┌──────────┐  ─────────────────────▶  ┌─────────┐
   │ 外部系统  │                          │ OpsMini │
   │ (监控/脚本 │  ◀─────────────────────  │ (单机面板)│
   │ /编排工具) │       统一 JSON 响应      └─────────┘
   └──────────┘
```

- **장기 연결 없음**：모두 표준 REST로, WebSocket / gRPC 장기 연결을 유지하지 않음；
- **스트리밍 시나리오**（로그 tail, 실시간 지표）：REST 페이지네이션 폴링으로 충분하며 SSE 불필요；
- **멱등**：조회용 GET은 자연히 멱등. 쓰기 작업(명령 실행, 리소스 생성)은 명확한 결과 반환.

### 7.3 주요 흐름

1. 패널 시작 → `agent_config`（token + 명령 허용 목록）읽기；
2. 외부 시스템이 `Authorization: Bearer <token>`을 포함해 `/agent/v1/*` 호출；
3. 미들웨어가 token 검증（상수 시간 비교）→ 허용 목록 일치 시 통과, 아니면 401；
4. 고위험 작업(명령 실행)은 명령 허용 목록 2차 검증을 거치고 모두 감사 기록；
5. 통합 JSON 반환：`{ code, message, data }`。

### 7.4 보안

- **인증**：패널은 독립적인 랜덤 Agent Token을 설정（암호화 저장）, 요청 헤더 `Authorization: Bearer <token>`；
- **전송**：HTTPS. 고보안 시나리오에서는 IP 허용 목록 추가 가능；
- **명령 허용 목록**：`/commands`는 명시적으로 허가된 명령만 허용하고 모두 감사 기록；
- **최소 노출**：`/agent/v1`을 `/api/v1`과 분리, Agent API는 UI 기능과 터미널을 노출하지 않음。

### 7.5 두 API의 포지셔닝

| 차원 | 패널 API `/api/v1` | Agent API `/agent/v1` |
|------|-------------------|----------------------|
| 사용자 | 브라우저（사람） | 외부 시스템（머신） |
| 인증 | 사용자 세션 JWT + RBAC | Agent Token |
| 범위 | 전체 UI 기능 + 터미널 WS | 리소스 관리 + 명령 실행（UI/터미널 없음） |
| 설계 | 인터랙션 지향 | 자동화 지향（멱등, 재시도 가능） |

---

## 8. 보안 설계

| 차원 | 방안 |
|------|------|
| 비밀번호 | bcrypt 해시 |
| 세션 | 단기 access JWT（15min）+ refresh token（취소 가능） |
| 2FA | TOTP（RFC 6238）, 로그인 시 선택적 2차 인증 |
| 인가 | RBAC 3역할：admin（전체）/ operator（일상 운영）/ readonly（읽기 전용） |
| 보안 진입점 | 패널 접근 시 secret 경로 필요（예 `/opsmini_panel`）, 포트 스캔 방지 |
| 키 저장 | 패널 키（API Key, Agent Token）AES-GCM 암호화 저장 |
| 터미널 | Web SSH는 operator 이상만 허용, 세션 기록 감사 |
| 무차별 대입 방지 | 로그인 실패 속도 제한 + 잠금 |
| 감사 | 중요 작업을 `audit_logs`에 기록 |
| CSRF/XSS | API는 쿠키 없는 Bearer Token 사용. 프런트엔드는 이스케이프 |

---

## 9. 주요 흐름

### 9.1 로그인

```
输入账密 → bcrypt 校验 → 若开启 2FA 则要求 TOTP
  → 签发 access + refresh → 前端存 refresh（httpOnly/localStorage）
  → 后续请求带 Bearer access → 过期用 refresh 换新
```

### 9.2 Web SSH 터미널

```
前端 ws://host/api/v1/terminal/ws?token=...
  → 服务端校验 token + 角色
  → 启动 pty（github.com/creack/pty）→ 双向数据转发
  → 关闭时回收 pty、记录会话时长
```

### 9.3 크론 작업 실행

- **패널 작업**：robfig/cron 상주 스케줄링, `cron_jobs`에 기록；
- **시스템/사용자 작업**：`/etc/crontab`, `/etc/cron.d/`, `/var/spool/cron/<user>`을 직접 읽기/쓰기（읽기 전용 표시 + 제어된 편집）.

### 9.4 AI 어시스턴트

```
用户输入 → Service 拼上下文（当前页面/模块 + 系统状态）
  → 调 LLM（provider 适配：OpenAI/DeepSeek/Qwen/Ollama）
  → 若模型判定为「执行意图」→ 生成结构化 action + 参数
  → 命中权限白名单 → 执行 → 回传结果
  → 未授权/高危 → 要求用户二次确认
```

### 9.5 지표 수집

- 수집기：gopsutil이 5초마다 샘플링 → 메모리 링 버퍼；
- 이력：다운샘플링 후 SQLite에 저장（1분 단위 7일 보존）；
- 푸시：WebSocket으로 대시보드/모니터링 페이지 구독자에게 브로드캐스트.

---

## 10. 모니터링과 관측성

- 구조화 로그（zerolog）, 레벨 구분, 패널 내 "로그" 페이지에서 확인 가능；
- 지표：패널 자체 + 호스트 지표를 monitor 모듈이 통합 수집；
- 헬스 체크：`/api/healthz`가 프로세스/DB/디스크 상태 반환.

---

## 11. 배포

### 11.1 산출물

- `opsmini` 단일 바이너리（약 25~30MB, `-s -w` 압축 후）, 프런트엔드, SQLite, 정적 리소스 내장；
- 설정：`/etc/opsmini/config.yaml` 또는 환경 변수；
- 기본 포트 8888, 데이터 디렉터리 `/var/lib/opsmini/`（opsmini.db）.

### 11.1.1 다중 아키텍처 릴리스（x64 + arm64）

릴리스는 **Linux**의 **x86_64（amd64）**와 **ARM64（arm64）** 두 명령어 세트를 커버해야 하며, macOS 타깃은 개발 디버깅용으로만 컴파일합니다.

| 타깃 플랫폼 | 적용 시나리오 |
|----------|----------|
| `linux/amd64` | 주류 x64 서버（Intel/AMD, 클라우드 벤더 범용 기종） |
| `linux/arm64` | ARM 서버（Graviton, 라즈베리 파이, 쿤펑, 피티움 등） |
| `darwin/arm64` | Apple Silicon 개발 머신（로컬 디버깅） |
| `darwin/amd64` | Intel Mac（개발 디버깅） |

> OpsMini는 **Linux 호스트** 대상으로 Linux 설치 패키지만 릴리스합니다. macOS 타깃은 개발 디버깅 전용이며 전달 산출물이 아닙니다.

**핵심 전제**：데이터 계층은 순수 Go 드라이버 `glebarez/sqlite`（기반 `modernc.org/sqlite`）를 채택하여 **CGO 의존성 없음**. 따라서 `CGO_ENABLED=0`으로 어느 플랫폼에서든 원클릭으로 **정적 링크** 바이너리를 크로스 컴파일할 수 있으며, 아키텍처별 C 크로스 툴체인을 준비할 필요가 없습니다.

**빌드 방법**（`Makefile` 준비됨）：

```bash
make build        # 当前平台
make build-all    # 全平台交叉编译 → dist/
```

산출물 명명：`opsmini-<version>-<os>-<arch>`, 버전 번호는 `-ldflags -X main.version`으로 주입, 런타임에 `opsmini -version`으로 확인 가능.

실측 통과：4개 타깃 플랫폼 모두 컴파일 성공, `file`로 아키텍처 정확성 검증（ELF x86-64 / ELF aarch64 / Mach-O arm64 / Mach-O x86_64）, 모두 정적 링크.

### 11.2 서비스화

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

### 11.3 리버스 프록시（선택 사항）

Nginx가 80/443 → 8888을 리버스 프록시하며, 인증서는 패널 내 웹사이트 모듈 또는 외부 로드 밸런서가 관리합니다.

---

## 12. 개발 로드맵（마일스톤）

| 단계 | 내용 | 전달 |
|------|------|------|
| M1 골격 | Go 프로젝트, Gin 라우팅, SQLite, GORM 마이그레이션, 로그인/사용자/RBAC | 실행 가능한 최소 패널 |
| M2 시스템 기능 | gopsutil 지표, 프로세스/포트/디스크/네트워크, 대시보드+모니터링 | 단일 머신 모니터링 루프 |
| M3 리소스 관리 | 웹사이트(Nginx), 데이터베이스, 파일, 터미널(pty), 크론 작업 | 1Panel 핵심 대응 |
| M4 컨테이너 | Docker SDK：컨테이너/이미지/볼륨/네트워크 | 컨테이너 관리 |
| M5 AI | LLM 어댑터 계층, 컨텍스트, NL 실행, AI 어시스턴트 | 차별화 기능 |
| M6 Agent API | Agent가 표준 REST API（`/agent/v1`）노출, token 인증, 명령 허용 목록 | 외부 통합 기능 |
| M7 마무리 | i18n, 감사, 속도 제한, 테스트, 문서 | 프로덕션 준비 |

---

## 13. 확정 대기 결정 사항

1. **Agent API 인증 강도**：기본 Bearer Token. mTLS 상호 인증서 도입이 필요한가（더 안전하지만 배포는 더 무거움）？
2. **Agent API에 IP 허용 목록이 필요한가**：기본 비활성, Token만 의존. 다중 머신/공용망 시나리오에서 IP 제한을 추가할지？
3. **스트리밍 데이터**：로그 tail / 실시간 지표, 기본 REST 페이지네이션 폴링, 수용 가능한가？（v1.0은 SSE/장기 연결을 도입하지 않음）
4. **다중 머신 통합 관리（향후）**：후속으로 Proxy 센터를 도입하면 기존 `/agent/v1`을 재사용해 풀/스케줄링을 할지, 별도 프로토콜을 둘지？（v1.0 범위 밖, 메모용）
