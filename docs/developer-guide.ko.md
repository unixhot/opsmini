<div align="center">

# OpsMini Developer Guide

**개발자 가이드**

[English](developer-guide.md) · [简体中文](developer-guide.zh-CN.md) · [繁體中文](developer-guide.zh-TW.md) · [日本語](developer-guide.ja.md) · [한국어](developer-guide.ko.md) · [ไทย](developer-guide.th.md) · [Deutsch](developer-guide.de.md)

</div>

---

> 버전: v1.0.0 · 언어: 한국어 · [English](developer-guide.md)

본 가이드는 OpsMini 코드베이스를 심층적으로 설명합니다: 아키텍처, 디렉터리 구성, 각 모듈, API 설계, 데이터 모델, RBAC 및 확장 방법.

---

## 1. 개요

OpsMini는 **경량 단일 호스트 서버 관리 패널**(BaoTa / 1Panel과 유사)로, 다음 두 가지로 차별화됩니다:

1. **AI 기반 운영** — LLM 어댑터를 통해 자연어로 시스템을 관리할 수 있습니다(진단, 로그 분석, 명령 실행).
2. **깔끔한 REST 계약** — 패널은 `/api/v1`(브라우저 UI용)과 `/agent/v1`(기기 간 통신)을 노출하여 중앙 프록시가 메트릭을 가져와 여러 호스트를 관리할 수 있습니다.

**단일 정적 바이너리**로 제공됩니다: Vue3 프런트엔드는 `go:embed`로 내장되고, SQLite는 순수 Go 드라이버(CGO 없음)를 사용하므로 `CGO_ENABLED=0` 교차 컴파일이 linux/amd64, linux/arm64, darwin, windows에서 동작합니다.

### 기술 스택

| 계층 | 기술 |
|------|------|
| 백엔드 | Go, Gin, GORM, `glebarez/sqlite`(순수 Go), gopsutil/v4, JWT(golang-jwt/v5), bcrypt, robfig/cron/v3 |
| 프런트엔드 | Vue 3(단일 파일 SPA), ECharts, xterm.js(웹 터미널) |
| 스토리지 | SQLite |
| 배포 | 단일 바이너리(`go:embed` 프런트엔드) |

---

## 2. 아키텍처

백엔드는 깔끔한 계층형 아키텍처를 따릅니다:

```
HTTP 요청
   │
   ▼
router(gin.Engine, 라우트 등록 + 미들웨어 배선)
   │
   ▼
middleware(인증 → RBAC 권한 → 감사 → 액세스 로그)
   │
   ▼
api/v1 handler(요청 파싱/검증, service 호출, 응답 작성)
   │
   ▼
service(비즈니스 로직, 오케스트레이션, 외부 시스템)
   │
   ▼
repository(GORM 데이터 액세스)
   │
   ▼
model(SQLite 테이블에 매핑된 구조체)
```

규칙:

- **handler**는 요청 파싱/검증과 응답 구성만 담당하며 비즈니스 로직은 포함하지 않습니다.
- **service**는 비즈니스 로직을 포함하고 repository와 시스템 리소스(gopsutil, Docker SDK, crontab, pty)를 오케스트레이션합니다.
- **repository**는 GORM을 래핑하며 데이터베이스에 접근하는 유일한 계층입니다.
- **model**은 테이블 매핑과 JSON 태그를 가진 GORM 구조체입니다.

### 요청 흐름(인증된 패널 API)

```
브라우저 ──► /api/v1/xxx
             │  authMw      (JWT 파싱, 사용자 id/role 주입)
             │  permMw      (RBAC 권한 검사, 라우트별 선택)
             │  auditMw     (감사 로그 기록)
             │  accessLogMw (액세스 로그 기록)
             ▼
          handler → service → repository → SQLite
```

Agent API(`/agent/v1/*`)는 사용자 JWT가 아닌 **별도의 Agent Token** 인증을 사용하며 `middleware.AgentAuth`로 보호됩니다.

---

## 3. 디렉터리 구조

```
opsmini/
├── cmd/
│   └── agent/main.go        # 진입점: 플래그 파싱, 설정 로드, DB 초기화, HTTP 시작
├── configs/
│   └── config.yaml          # 기본 설정 템플릿
├── internal/
│   ├── config/              # 설정 로드(Config 구조체 + YAML + 기본값)
│   ├── router/              # gin 라우터, 라우트 등록, 미들웨어 배선
│   ├── middleware/          # auth, perm(RBAC), audit, accesslog, cors, ratelimit, metrics_auth, agent
│   ├── api/v1/              # HTTP 핸들러(패널 + agent 엔드포인트)
│   ├── service/             # 비즈니스 로직(모듈당 파일 하나)
│   ├── repository/          # GORM 데이터 액세스(모델당 파일 하나)
│   ├── model/               # GORM 모델 + RBAC 권한 그룹
│   └── pkg/
│       ├── jwt/             # JWT 서명/검증(access + refresh)
│       ├── response/        # 통합 API 응답 봉투
│       └── store/           # SQLite 초기화, 자동 마이그레이션, 시드 데이터
├── web/
│   ├── index.html           # 단일 파일 Vue3 SPA(7개 언어 i18n, 테마)
│   ├── embed.go             # go:embed로 프런트엔드를 바이너리에 내장
│   └── static/              # 오프라인 프런트엔드 의존성
├── install/                 # 설치 스크립트
├── docs/                    # 프로젝트 문서
└── Makefile                 # 빌드 / 교차 컴파일 / 버전 타깃
```

---

## 4. 모듈 참조

### 4.1 `cmd/agent/main.go`

진입점. 책임:

- 플래그 파싱(`-config`, `-version`);
- `internal/config`를 통한 설정 로드;
- `internal/pkg/store`를 통한 SQLite 열기;
- 내장 역할과 기본 admin 계정 시드;
- service + handler를 구성하여 `internal/router`에 전달;
- HTTP 서버 시작(및 선택적 metrics 엔드포인트).

### 4.2 `internal/config`

`config.go`는 `Config` 구조체를 정의하고 `-config` 경로에서 YAML을 로드합니다. 파일이 없으면 내장 기본값이 반환됩니다. 섹션: `server`, `database`, `jwt`, `ai`, `agent`.

### 4.3 `internal/router`

`router.go`는 모든 라우트가 등록되는 단일 지점입니다:

- 공개 라우트(`/healthz`, `/auth/login`, `/auth/refresh`, ...);
- 인증된 패널 라우트(`/api/v1/*`)는 `authMw + audit + accesslog` 뒤에 위치;
- 쓰기 라우트는 추가로 `permMw("permission.key")`로 보호;
- agent 라우트(`/agent/v1/*`)는 `middleware.AgentAuth` 뒤에 위치;
- WebSocket 라우트(`/terminal`, `/containers/:id/exec`).

**규약:** 모든 쓰기 엔드포인트(POST/PUT/DELETE)는 `permMw(...)` 가드를 가져야 합니다. 읽기 엔드포인트는 민감한 데이터를 노출하지 않는 한 로그인한 모든 사용자에게 열려 있습니다.

### 4.4 `internal/middleware`

| 파일 | 목적 |
|------|------|
| `auth.go` | JWT 인증. 사용자 id/role을 컨텍스트에 주입 |
| `perm.go` | RBAC 권한 검사(`permMw`) |
| `audit.go` | 감사 로그 항목 기록 |
| `accesslog.go` | 액세스 로그 항목 기록 |
| `cors.go` | CORS 헤더 |
| `ratelimit.go` | 로그인 속도 제한 |
| `metrics_auth.go` | `/metrics`의 Bearer Token 가드 |
| `agent.go` | `/agent/v1`의 Agent Token 인증 |

### 4.5 `internal/api/v1`

모듈당 하나의 핸들러 파일. 각 핸들러:

1. 요청을 바인딩/검증;
2. 해당 service 메서드 호출;
3. 통합된 `response.OK` / `response.Error` 반환.

패널 핸들러는 `v1` 패키지에 있습니다(예: `system.go`, `file.go`, `skill.go`). agent 핸들러는 `agent.go`에 있습니다.

### 4.6 `internal/service`

비즈니스 로직 계층. 모듈당 하나의 파일. 주요 모듈:

| 파일 | 모듈 |
|------|------|
| `auth.go`, `user.go`, `role.go`, `totp.go` | 인증, 사용자, RBAC, 2FA |
| `system.go`, `metrics.go`, `prometheus.go` | 호스트 정보, 메트릭, prometheus 내보내기 |
| `website.go`, `database.go`, `cron.go`, `crontab.go` | 리소스 관리 |
| `file.go` | 경로 탐색 보호가 있는 파일 작업 |
| `docker.go` | Docker 컨테이너/이미지/볼륨/네트워크 |
| `ai.go` | LLM 어댑터(채팅, 스트리밍) |
| `skill.go`, `mcp.go` | AI 스킬 + MCP 서버 |
| `alert.go`, `alertmonitor.go` | 경고 규칙 + 평가 |
| `security.go`, `securitymonitor.go`, `baseline.go`, `fim.go`, `threat.go`, `firewall.go`, `loginsecurity.go` | 호스트 보안 스위트 |
| `notification.go`, `audit.go`, `setting.go` | 알림, 감사, 설정 |
| `appstore.go`, `apptemplate.go`, `appcategory.go` | 앱 스토어 / 템플릿 |

### 4.7 `internal/repository`

GORM 데이터 액세스. 모델당 하나의 파일. CRUD와 쿼리 헬퍼를 제공합니다. 비즈니스 로직은 포함하지 않습니다.

### 4.8 `internal/model`

SQLite 테이블에 매핑된 GORM 구조체와, RBAC 권한 그룹의 단일 권위 소스인 `role.go`(`PermGroups()`, `AllPermKeys()`, `BuiltinRoles()`).

### 4.9 `internal/pkg`

| 패키지 | 목적 |
|--------|------|
| `jwt` | access/refresh 토큰 서명 및 검증 |
| `response` | 통합 응답 봉투 `{code,message,data}` |
| `store` | SQLite 열기, 자동 마이그레이션, 시드(기본 admin) |

### 4.10 `web`

- `index.html` — 단일 파일 Vue3 SPA: 7개 언어 i18n, 테마 시스템, 로그인, 대시보드, 모니터링, 보안, 파일, 터미널, AI 어시스턴트, 설정.
- `embed.go` — `go:embed`로 프런트엔드를 바이너리에 내장.
- `static/` — 오프라인 프런트엔드 의존성(Vue, ECharts).

---

## 5. API 설계

### 5.1 응답 봉투

모든 엔드포인트가 반환합니다:

```json
{ "code": 0, "message": "ok", "data": { } }
```

- `code == 0` → 성공, `data`가 페이로드를 보유;
- `code != 0` → 비즈니스 오류, `message`가 설명.

### 5.2 인증

- 패널 API(`/api/v1`): JWT 액세스 토큰(`Authorization: Bearer <token>`), 수명이 짧으며 `/auth/refresh`로 갱신.
- Agent API(`/agent/v1`): 정적 Agent Token(설정 `agent.token`).

### 5.3 엔드포인트 패밀리

| 패밀리 | 대상 | 인증 |
|--------|------|------|
| `/api/v1/*` | 브라우저 UI | 사용자 JWT + RBAC |
| `/agent/v1/*` | 외부 시스템 / 프록시 | Agent Token |

---

## 6. 데이터 모델

모델은 `internal/model` 내의 GORM 구조체입니다. 테이블은 시작 시 `internal/pkg/store`에 의해 자동 마이그레이션됩니다. 대표 모델:

- `User`(id, username, 비밀번호 해시, role, MFA 시크릿, ...)
- `Role`(name, label, perms, builtin)
- `Website`, `Database`, `CronJob`
- `AlertRule`, `AlertEvent`, `Notification`
- `McpServer`, `Skill`(스킬은 디스크의 디렉터리로 저장, DB 아님)
- `AuditLog`, `AccessLog`, `Setting`
- 보안: `BaselineResult`, `FimBaseline`, `FimChange`, `ThreatFinding`

---

## 7. RBAC 및 권한

권한 그룹은 `internal/model/role.go`(`PermGroups()`)에 정의되어 있으며, 단일 정보 소스입니다. 세 가지 내장 역할:

- **admin** — 권한 `"*"`(전체);
- **operator** — `user.*`와 `settings.edit`를 제외한 모든 권한;
- **readonly** — `*.view` 권한만.

프런트엔드 메뉴/버튼은 `hasPerm('key')`로 동일한 키를 기반으로 제어되고, 백엔드는 `permMw("key")`로 강제합니다. 새 쓰기 기능을 추가할 때 **반드시** 세 곳 모두 변경해야 합니다:

1. `PermGroups()`에 권한 키 추가;
2. `permMw(...)`로 라우트 보호;
3. `hasPerm(...)`로 버튼 보호.

---

## 8. 빌드 및 배포

```bash
make build          # 현재 플랫폼용 빌드
make build-all      # 모든 플랫폼 교차 컴파일
make version        # 버전 정보 출력
```

바이너리는 정적입니다(CGO 없음). systemd, 리버스 프록시, 업그레이드 지침은 [`build-and-deploy.md`](build-and-deploy.md)를 참조하세요.

---

## 9. 개발 가이드

### 9.1 새 모듈 추가

계층형 패턴을 따라 생성(또는 확장)합니다:

1. `internal/model/xxx.go` — GORM 구조체;
2. `internal/repository/xxx.go` — 데이터 액세스;
3. `internal/service/xxx.go` — 비즈니스 로직;
4. `internal/api/v1/xxx.go` — 핸들러;
5. `internal/router/router.go`에 라우트 등록.

### 9.2 새 쓰기 엔드포인트 추가

1. `internal/model/role.go`에 권한 키 추가;
2. `permMw("...")`로 라우트 등록;
3. 프런트엔드 버튼에 `hasPerm("...")` 가드 추가;
4. `web/index.html`에 i18n 키 추가(7개 언어 모두).

### 9.3 i18n 규약

프런트엔드 i18n 사전(`I18N`...`I18N8`)은 7개 언어를 보유합니다: `zh-CN`, `zh-TW`, `en`, `ja`, `ko`, `th`, `de`. 모든 새 키는 **7개 언어 모두**에 추가해야 합니다 — `t(key)` 헬퍼는 누락 시 `zh-CN`으로 폴백하지만, 번역이 누락되면 비중국어 사용자에게 중국어가 표시됩니다.

### 9.4 코드 스타일

- 내보내진 Go 함수/타입에는 해당 이름으로 시작하는 문서 주석을 답니다.
- 주석은 영어로 작성합니다.
- 각 디렉터리에는 해당 파일을 설명하는 `README.md`가 있습니다.
