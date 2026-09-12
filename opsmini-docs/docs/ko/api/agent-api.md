# Agent API（/agent/v1）

Agent API는 **머신 / 서드파티 시스템**을 위한 표준 REST 인터페이스로, 모니터링 플랫폼, 자동화 스크립트, 오케스트레이션 도구가 통합 호출할 수 있도록 제공됩니다.

## 패널 API와의 차이점

| 차원 | 패널 API `/api/v1` | Agent API `/agent/v1` |
|------|-------------------|----------------------|
| 사용자 | 브라우저(사람) | 외부 시스템(머신) |
| 인증 | 사용자 세션 JWT + RBAC | Agent Token（Bearer） |
| 범위 | 전체 UI 기능 + 터미널 WS | 리소스 조회 + 명령 실행（UI / 터미널 없음） |
| 설계 | 인터랙션 지향 | 자동화 지향（멱등성, 재시도 가능） |

## 인증

1. 패널「설정 → API Token」페이지에서 액세스 토큰을 생성합니다（토큰은 데이터베이스에 영속화되며 더 이상 `config.yaml`에 기록하지 않습니다）
2. 요청에 `Authorization: Bearer <token>`을 포함합니다
3. 미들웨어가 상수 시간 비교로 토큰을 검증합니다; 토큰이 비어 있으면 Agent API가 비활성화됩니다

## 엔드포인트 목록

| 메서드 | 경로 | 설명 |
|------|------|------|
| GET | `/agent/v1/health` | 헬스 체크（프로브） |
| GET | `/agent/v1/version` | 버전 정보 |
| GET | `/agent/v1/status` | 상태 요약: cpu / mem / 디스크 / 온라인 서비스 |
| GET | `/agent/v1/system/info` | 호스트 정보（hostname / os / 커널） |
| GET | `/agent/v1/system/processes` | 프로세스 목록 |
| GET | `/agent/v1/system/ports` | 포트 리스닝 |
| GET | `/agent/v1/system/disks` | 디스크 / 마운트 포인트 |
| GET | `/agent/v1/websites` | 웹사이트 목록 |
| GET | `/agent/v1/databases` | 데이터베이스 목록 |
| GET | `/agent/v1/cron-jobs` | 예약 작업 |
| GET | `/agent/v1/containers` | 컨테이너 목록 |
| POST | `/agent/v1/commands` | 명령 실행（화이트리스트） |
| POST | `/agent/v1/script/run` | 스크립트 실행 |
| POST | `/agent/v1/file/upload` | 파일 업로드 |

## 명령 실행과 화이트리스트

`/commands`는 명시적으로 허가된 명령 프리픽스만 실행할 수 있으며, 모두 감사(audit)에 기록됩니다. 화이트리스트는 `config.yaml`의 `agent.allowed_commands`에서 관리합니다:

```yaml
agent:
  allowed_commands:
    - ps
    - df
    - free
    - uptime
    - docker
    - systemctl
    - nginx
    - who
```

## 호출 예시

```bash
curl -H "Authorization: Bearer <token>" \
     https://<host>:8888/agent/v1/status
```

## 보안 권장 사항

- HTTPS 전송 사용
- 보안 요구가 높은 환경에서는 IP 화이트리스트로 출처를 제한할 수 있습니다
- 명령 화이트리스트를 최소화하여 필요한 명령만 허가하세요
- 토큰이 유출되면 즉시 패널「설정 → API Token」에서 재생성하세요
