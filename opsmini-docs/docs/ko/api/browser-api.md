# 패널 API（/api/v1）

패널 API는 브라우저 UI를 대상으로 하며, 사용자 세션 JWT + RBAC 인증을 사용합니다.

## 인증

- 로그인 인터페이스 `POST /auth/login`이 `access` + `refresh` 토큰을 반환합니다
- 이후 요청에는 `Authorization: Bearer <access_token>`을 포함합니다
- access가 만료되면 refresh로 새로 교체합니다
- 로그인 속도 제한（브루트포스 방지）; 2FA 활성화 후 로그인 시 먼저 `POST /auth/mfa/verify`로 동적 코드를 검증해야 합니다

## 통일된 응답

```json
{ "code": 0, "message": "ok", "data": { } }
```

`code`가 0이 아니면 비즈니스 오류입니다.

## 엔드포인트 목록

### 인증과 사용자

| 메서드 | 경로 | 설명 |
|------|------|------|
| POST | `/auth/login` | 로그인（속도 제한） |
| POST | `/auth/refresh` | 토큰 갱신 |
| POST | `/auth/logout` | 로그아웃 |
| POST | `/auth/mfa/verify` | 로그인 시 MFA 동적 코드 검증 |
| GET | `/auth/mfa/status` | 현재 사용자 MFA 바인딩 상태 조회 |
| POST | `/auth/mfa/setup` | MFA 바인딩 QR코드/시크릿 생성 |
| POST | `/auth/mfa/enable` | 검증 후 MFA 활성화 |
| POST | `/auth/mfa/disable` | MFA 해제 |
| GET | `/profile` | 개인 프로필（닉네임/아바타/이메일） |
| PUT | `/profile` | 개인 프로필 수정 |
| POST | `/profile/password` | 비밀번호 변경 |
| GET/POST/PUT/DELETE | `/users` | 사용자 CRUD |
| GET/POST/PUT/DELETE | `/roles` | 역할 CRUD |
| GET | `/roles/groups` | 권한 그룹 |
| GET | `/permissions` | 현재 사용자 권한 |

### 패널 설정

| 메서드 | 경로 | 설명 |
|------|------|------|
| GET | `/settings` | 모든 비민감 설정 읽기（`settings.view`） |
| PUT | `/settings` | 설정 일괄 업데이트（`settings.edit`） |

> 민감 항목（`jwt_secret`、`metrics_pass`）은 반환되지 않습니다; 쓰기 보호 항목（`jwt_secret`）은 덮어쓸 수 없습니다. 자세한 내용은 [패널 설정](../configuration/panel-settings.md)을 참조하세요.

### 모니터링과 알림

| 메서드 | 경로 | 설명 |
|------|------|------|
| GET | `/dashboard/overview` | 대시보드 개요 |
| GET | `/dashboard/metrics` | 대시보드 지표 |
| GET | `/system/monitor` | 모니터링 요약 |
| CRUD | `/alert-rules` | 알림 규칙 |
| GET | `/alert-events` | 알림 이벤트 |

### 알림

| 메서드 | 경로 | 설명 |
|------|------|------|
| GET | `/notifications` | 알림 목록 |
| GET | `/notifications/unread-count` | 읽지 않은 개수 |
| PUT | `/notifications/read-all` | 모두 읽음 처리 |
| PUT | `/notifications/:id/read` | 읽음 표시 |
| DELETE | `/notifications/:id` / `/notifications` | 개별 삭제 / 전체 비우기 |

### 리소스 관리

| 메서드 | 경로 | 설명 |
|------|------|------|
| CRUD | `/websites` | 웹사이트 |
| CRUD | `/databases` | 데이터베이스 |
| GET/POST | `/apps` `/app-categories` | 앱 스토어와 분류 |
| GET/POST | `/containers` `/images` `/volumes` `/networks` | 컨테이너 네 가지 |
| GET/POST | `/files` | 파일 |
| CRUD | `/cron-jobs` | 예약 작업 |
| GET | `/logs` `/logs/tail` | 로그 |

### 시스템과 보안

| 메서드 | 경로 | 설명 |
|------|------|------|
| GET | `/system/info` `/processes` `/ports` `/disks` `/network` `/users` `/groups` `/firewall` | 시스템 리소스 |
| GET/POST | `/security/...` | 호스트 보안（베이스라인 / FIM / 위협 / 방화벽 / 로그인 보안） |
| GET | `/audit-logs` `/access-logs` | 감사 / 액세스 로그 |

### AI와 통합

| 메서드 | 경로 | 설명 |
|------|------|------|
| POST | `/ai/chat` | AI 대화（함수 호출） |
| POST | `/ai/chat/stream` | AI 대화（스트리밍 SSE） |
| GET/POST/PUT/DELETE | `/mcp` | MCP 설정 |
| GET/POST/DELETE | `/skills` 등 | AI 스킬（SkillHub 검색/설치/업로드） |

### 터미널

| 메서드 | 경로 | 설명 |
|------|------|------|
| GET | `/terminal` | WebSocket 터미널（query token 인증） |
| GET | `/containers/:id/exec` | 컨테이너 WebSocket 터미널 |

## 설명

- 모든 쓰기 작업은 RBAC 권한 포인트의 제어를 받습니다（예: `website.create`、`container.edit`、`settings.edit`）
- 주요 작업은 감사 로그에 기록되고, 로그인 상태 요청은 액세스 로그에 기록됩니다
