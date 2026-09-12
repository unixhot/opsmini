# 빠른 시작

OpsMini를 사용해 주셔서 감사합니다 —— 경량 Linux 호스트 운영 패널입니다. 이 가이드는 최단 시간 안에 설치, 로그인을 완료하고 핵심 기능을 이해하도록 돕습니다.

## 여기서 시작하세요

<div class="grid cards" markdown>

-   :material-rocket-launch: **[5분 빠른 설치](quick-install.md)**

    ---

    한 줄 명령으로 배포 완료, 최초 시작 시 관리자 계정과 랜덤 비밀번호 자동 생성.

-   :material-login: **[최초 로그인](first-steps.md)**

    ---

    패널 로그인, 비밀번호 변경, 2단계 인증（MFA）바인딩.

-   :material-information-outline: **[제품 소개](introduction.md)**

    ---

    OpsMini의 포지셔닝, 핵심 기능과 기술 스택 이해.

</div>

## 핵심 개념

| 개념 | 설명 |
|------|------|
| **단일 머신 패널** | 각 호스트가 독립적으로 하나의 OpsMini 인스턴스를 실행하며 중앙 노드에 의존하지 않음 |
| **단일 바이너리 제공** | 프런트엔드 UI를 `go:embed`로 임베드, 배포는 실행 파일 하나를 복사하는 것 |
| **이중 API** | `/api/v1`은 브라우저 UI 대상（JWT + RBAC）, `/agent/v1`은 머신/서드파티 통합 대상（Token + 명령 화이트리스트） |
| **AI 대형 모델 운영** | 자연어 진단, 로그 분석, 명령 실행, OpenAI / DeepSeek / Qwen / Ollama 연동 |

## 다음 단계

설치를 완료한 후 다음 순서로 읽는 것을 권장합니다:

1. [설치 배포](../installation/index.md) — 시스템 요구 사항, 원클릭 설치, 수동 배포, 리버스 프록시
2. [설정 가이드](../configuration/index.md) — 설정 파일, 데이터 저장, HTTPS
3. [기능 가이드](../features/index.md) — 대시보드, AI 어시스턴트, 앱 관리, 호스트 보안 등
4. [REST API](../api/index.md) — 대외 통합 인터페이스
