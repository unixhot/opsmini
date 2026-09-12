---
title: OpsMini — 경량 Linux 호스트 운영 패널
hide:
  - navigation
  - toc
---

<div class="home-hero" markdown>

# OpsMini

**경량 Linux 호스트 운영 패널** — 내장 AI 대형 모델 운영, 표준 REST API 대외 통합

바오타 / 1Panel에 대응하며, 단일 머신 운영을 더 간단하고, 더 스마트하고, 더 통합 가능하게 만듭니다.

<div class="home-cta" markdown>
[빠른 설치](getting-started/quick-install.md){ .md-button .md-button--primary }
[문서 보기](getting-started/index.md){ .md-button }
[:fontawesome-brands-github: GitHub](https://github.com/unixhot/opsmini){ .md-button }
</div>

</div>

---

## 두 가지 핵심 차별화

<div class="grid cards" markdown>

-   :material-robot-outline: **AI 대형 모델 운영**

    ---

    자연어로 장애 진단, 로그 분석, 운영 명령 실행. OpenAI / DeepSeek / Qwen / Ollama 연동으로 AI를 당신의 운영 부조종사로 만드세요.

-   :material-api: **표준 REST API**

    ---

    내장 `/api/v1`（패널 API）과 `/agent/v1`（머신 간 API）, 모든 모니터링 플랫폼, 자동화 스크립트, 오케스트레이션 도구가 바로 통합 호출할 수 있습니다.

</div>

---

## 기능 목록

<div class="grid cards" markdown>

-   :material-view-dashboard-outline: **대시보드와 모니터링**

    ---

    실시간 CPU / 메모리 / 디스크 / 네트워크 시계열, 알림 규칙과 알림 이벤트 폐루프.

-   :material-docker: **컨테이너 관리**

    ---

    Docker 컨테이너, 이미지, 볼륨, 네트워크 네 가지 리소스 전체 관리, 앱 스토어로 일반 앱 원클릭 설치.

-   :material-console: **웹 터미널**

    ---

    WebSocket + pty로 구현한 SSH 유사 인터랙티브 터미널, 브라우저에서 바로 서버 조작.

-   :material-shield-check-outline: **호스트 보안**

    ---

    베이스라인 검사, 파일 무결성 모니터링（FIM）, 위협 탐지, 방화벽, 로그인 보안, 보안 태세를 한 화면에서 파악.

-   :material-folder-outline: **파일과 웹사이트**

    ---

    파일 탐색 / 업로드 / 편집, Nginx 웹사이트, 데이터베이스, SSL 인증서 원스톱 관리.

-   :material-translate: **7개 언어 · 단일 바이너리**

    ---

    간체/번체 중국어, 영어, 일본어, 한국어, 태국어, 독일어 7개 언어 인터페이스; 프런트엔드 `go:embed` 임베드, 약 30MB 정적 링크, 런타임 의존성 제로.

</div>

---

## 원클릭 설치

<div class="home-section" markdown>

### 5분 시작

```bash
# Linux x86_64 / aarch64, 원클릭 스크립트로 /data/opsmini에 설치, 포트 8888
curl -fsSL https://opsmini.com/install.sh | sudo bash
```

최초 시작 시 관리자 계정 `opsmini`와 랜덤 비밀번호가 자동 생성되며（시작 로그 참조）, `http://<host>:8888`을 열면 됩니다.

[전체 설치 문서 보기 →](installation/index.md){ .md-button }

</div>

---

<div class="home-section" markdown>

## 바로 시작하기

한 줄 명령으로 배포, AI로 운영을 강화하고, 표준 API로 통합을 연결합니다.

[설치 시작](getting-started/quick-install.md){ .md-button .md-button--primary }
[문서 탐색](getting-started/index.md){ .md-button }

</div>
