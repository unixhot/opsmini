# 디렉터리 구조

## 계층

각 모듈 내부는 엄격하게 3계층으로 구성됩니다:

```
Controller（HTTP handler）：파라미터 파싱·검증, Service 호출, 응답 조립
    → Service（비즈니스 로직）：Repository와 시스템 리소스 오케스트레이션, 트랜잭션 경계
    → Repository（GORM）：순수 데이터 액세스, 비즈니스 로직 없음
```

## 디렉터리 구성

```
opsmini/
├── cmd/
│   └── agent/main.go          # 메인 프로그램 진입점
├── internal/
│   ├── router/                # 라우트 등록, API 버전
│   ├── middleware/            # 인증, 권한, 감사, 속도 제한, 보안 진입점
│   ├── api/v1/                # Controller（모듈별 패키지 분리）
│   ├── service/               # Service（모듈별 패키지 분리）
│   ├── repository/            # Repository（모듈별 패키지 분리）
│   ├── model/                 # GORM 데이터 모델
│   ├── config/                # 설정 로딩
│   └── pkg/                   # 공용 유틸（jwt/response/store）
├── web/                       # Vue3 프런트엔드（빌드 후 embed）
│   ├── index.html             # SPA 진입점
│   └── static/                # echarts/vue/xterm 등 정적 라이브러리
├── configs/                   # 기본 설정 예시
├── docs/                      # 문서
└── install/                   # 원클릭 설치 스크립트
```

## 핵심 모듈

| 모듈 | 책임 |
|------|------|
| `auth` | 로그인 / 로그아웃, 세션, 2FA, RBAC |
| `setting` | 패널 설정, 테마, 메뉴 표시/숨김, 언어 |
| `dashboard` / `monitor` | 지표 집계, 시계열 수집, 알림 규칙 |
| `website` / `database` / `appstore` | 웹사이트, 데이터베이스, 앱 스토어 |
| `container` | Docker 컨테이너 / 이미지 / 볼륨 / 네트워크 |
| `system` / `file` / `terminal` | 시스템 리소스, 파일, 웹 터미널 |
| `cron` / `log` | 예약 작업, 로그 |
| `ai` | LLM 연동, 컨텍스트, 자연어 실행 |
| `security` | 베이스라인, FIM, 위협, 방화벽, 로그인 보안 |
| `agent` | 대외 REST API（`/agent/v1`） |
