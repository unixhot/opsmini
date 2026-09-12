# 메인 설정 파일

OpsMini는 YAML 설정 파일을 사용합니다. 공식 원클릭 설치 후 기본 위치는 `/data/opsmini/config.yaml`이며, 소스 코드로 실행할 때는 기본적으로 `configs/config.yaml`을 사용합니다. 둘 다 `-config` 파라미터로 지정할 수 있습니다. 설정 파일이 존재하지 않으면 프로그램은 내장 기본값으로 시작합니다.

공식 배포에 포함된 예시는 이미 **최소 실행 가능 설정**으로 간소화되었으며, 나머지 런타임 설정（AI, Agent 토큰, 모니터링 인증 등）은 패널「설정」페이지로 이전되었습니다.

```yaml
# OpsMini Agent 설정 파일
server:
  host: "0.0.0.0"        # 리스닝 주소
  port: 8888              # 리스닝 포트
  secret_entry: ""        # 보안 진입 경로 프리픽스, 예: /opsmini_panel（비워 두면 비활성화）

database:
  path: "opsmini.db"      # SQLite 데이터 파일 경로

jwt:
  access_ttl_seconds: 900     # access token 유효 기간（15분）
  refresh_ttl_seconds: 604800 # refresh token 유효 기간（7일）

log:
  level: error               # 로그 레벨: info / warn / error
  path: ""                   # 로그 파일 경로, 비워 두면 stdout으로만 출력（systemd journal）
```

## 설정 항목 설명

### server

| 파라미터 | 설명 | 기본값 |
|------|------|------|
| `host` | 리스닝 주소, `0.0.0.0`은 모든 네트워크 인터페이스를 의미 | `0.0.0.0` |
| `port` | 리스닝 포트 | `8888` |
| `secret_entry` | 보안 진입 경로 프리픽스, 예: `/opsmini_panel`, 비워 두면 비활성화 | 비어 있음 |

`secret_entry`를 설정하면 패널과 모든 API가 프리픽스 경로 아래에 마운트됩니다（예: `http://<host>:8888/opsmini_panel`）. 리버스 프록시와 함께 사용하면 실제 진입점을 숨겨 포트 스캔과 무차별 탐지를 방지할 수 있습니다.

### database

| 파라미터 | 설명 | 기본값 |
|------|------|------|
| `path` | SQLite 데이터 파일 경로 | `opsmini.db` |

### jwt

| 파라미터 | 설명 | 기본값 |
|------|------|------|
| `access_ttl_seconds` | access token 유효 기간（초） | `900` |
| `refresh_ttl_seconds` | refresh token 유효 기간（초） | `604800` |

> JWT 서명 키는 **더 이상** 여기에서 설정하지 않습니다. 최초 시작 시 32바이트 랜덤 키를 자동 생성하여 데이터베이스에 영속화하며, 설정 파일에 기록하지도, 인터페이스에 표시하지도 않으므로 수동으로 관리할 필요가 없습니다.

### log

| 파라미터 | 설명 | 기본값 |
|------|------|------|
| `level` | 로그 레벨: `info` / `warn` / `error` | `error` |
| `path` | 로그 파일 경로, 비워 두면 stdout으로만 출력（systemd journal） | 비어 있음 |

로그 레벨은 출력 상세도를 제어합니다: `error`는 오류만 출력（프로덕션 권장, SQL 쿼리 로그 도배 방지）; `warn`은 느린 쿼리와 경고를 추가 출력; `info`는 모든 로그를 출력（SQL 쿼리 포함, 문제 해결에 적합）. 원클릭 설치 시 기본적으로 `/data/opsmini/opsmini.log`에 기록합니다.

## 선택적 설정 섹션

다음 필드는 설정 구조에서 여전히 지원되지만, 공식 예시에서는 생략되어 있습니다（내장 기본값 사용）. 필요에 따라 명시적으로 선언할 수 있습니다.

### agent（명령 화이트리스트）

```yaml
agent:
  allowed_commands:        # Agent API 명령 화이트리스트 프리픽스
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

| 파라미터 | 설명 | 기본값 |
|------|------|------|
| `allowed_commands` | `/agent/v1/commands`에서 실행할 수 있는 명령 프리픽스 화이트리스트 | 위 참조 |

> `agent.token`은 폐기되었습니다. Agent API의 인증 토큰은 패널「설정 → API Token」페이지에서 생성·관리하며, 비워 두면 Agent API가 비활성화됩니다.

### metrics（Prometheus 지표）

```yaml
metrics:
  enabled: true     # /metrics 엔드포인트 활성화 여부
  user: ""          # Basic 인증 사용자 이름, 비어 있으면 인증 없음
  password: ""      # Basic 인증 비밀번호
```

| 파라미터 | 설명 | 기본값 |
|------|------|------|
| `enabled` | `/metrics` 엔드포인트 활성화 여부 | `true` |
| `user` | HTTP Basic 인증 사용자 이름, 비어 있으면 인증 없음 | 비어 있음 |
| `password` | HTTP Basic 인증 비밀번호 | 비어 있음 |

> 인증 자격 증명 우선순위: 패널「설정 → 모니터링 내보내기」의 사용자 이름/비밀번호가 여기의 `user`/`password`보다 **우선합니다**. 사용자 이름이 비어 있으면 공개 액세스입니다.

## 패널 설정으로 이전된 설정

다음 설정 항목은 `config.yaml`에서 제외되어 패널「설정」페이지에서 통합 관리됩니다（SQLite에 저장, 런타임 즉시 적용）:

| 원래 설정 | 현재 관리 위치 | 설명 |
|--------|-----------|------|
| `jwt.secret` | 자동 생성（관리 불필요） | 최초 시작 시 랜덤 키 생성 후 DB 저장 |
| `ai.*` | 설정 → AI 대형 모델 연동 | 모델 / API Key / Base URL / 활성화 스위치 |
| `agent.token` | 설정 → API Token | Agent API 액세스 토큰, 비워 두면 비활성화 |
| `metrics.user/password` | 설정 → 모니터링 내보내기 | 런타임에 설정 파일 값 덮어쓰기 가능 |

자세한 내용은 [패널 설정](panel-settings.md)을 참조하세요.

## 환경 변수

| 변수 | 설명 |
|------|------|
| `OPSMINI_AI_KEY` | AI API Key, **최우선 순위**（패널 설정과 설정 파일보다 높음, 키의 디스크 저장 방지） |

## 명령줄 파라미터

| 파라미터 | 설명 |
|------|------|
| `-config <path>` | 설정 파일 경로 지정（원클릭 설치 기본값 `/data/opsmini/config.yaml`, 소스 실행 기본값 `configs/config.yaml`） |
| `-version` | 버전 정보 출력 후 종료 |
| `-reset-mfa <username>` | 지정 사용자의 MFA 바인딩 초기화（인증 코드 분실 시 사용）, 완료 후 종료 |
| `-reset-pass <username>` | 지정 사용자의 비밀번호를 랜덤 강력 비밀번호로 초기화하고 출력, 완료 후 종료 |

> `-reset-mfa` / `-reset-pass`는 데이터베이스를 직접 조작하며 서비스를 시작하지 않습니다. 계정 분실 복구용입니다.
