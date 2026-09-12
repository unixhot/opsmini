# REST API

OpsMini는 브라우저 UI와 외부 시스템 통합 호출을 위해 두 가지 표준 REST API를 대외적으로 노출합니다.

## API 개요

| API | 프리픽스 | 사용자 | 인증 |
|-----|------|--------|------|
| 패널 API | `/api/v1` | 브라우저（사람） | 사용자 세션 JWT + RBAC |
| Agent API | `/agent/v1` | 외부 시스템（머신） | Agent Token + 명령 화이트리스트 |
| Prometheus 지표 | `/metrics` | 모니터링 시스템 | 선택적 Basic 인증 |

통일된 응답 형식: `{ "code": 0, "message": "ok", "data": ... }`, `code`가 0이 아니면 비즈니스 오류입니다.

<div class="grid cards" markdown>

-   :material-web: **[패널 API（/api/v1）](browser-api.md)**

    ---

    인증, 사용자, 모니터링, 웹사이트, 컨테이너, 파일, AI 등 모든 UI 기능.

-   :material-api: **[Agent API（/agent/v1）](agent-api.md)**

    ---

    머신 간 리소스 관리 및 명령 실행 인터페이스.

-   :material-chart-line: **[Prometheus /metrics](prometheus.md)**

    ---

    node_exporter 호환 지표, Prometheus 직접 scrape.

</div>
