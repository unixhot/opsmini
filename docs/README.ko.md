<div align="center">

# OpsMini

**서버 위에서 사는 AI 운영 에이전트**

[English](README.en.md) · [简体中文](../README.md) · [繁體中文](README.zh-TW.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [ไทย](README.th.md) · [Deutsch](README.de.md)

</div>

---

## 한국어

OpsMini 는 Linux 호스트에 상주하는 AI 운영 에이전트입니다. 지표를 모으고 이상을 판단하며 직접 처리하고, 위험한 작업 앞에서는 승인을 요청합니다. 웹 UI 는 관측 창일 뿐이며, 아무것도 클릭하지 않아도 매일 작업을 끝내고 보고서를 남깁니다. 수십 MB 단일 정적 바이너리로 동작하며 데이터는 모두 로컬에 남습니다.

### 기능

- **AI 운영 지원** — 자연어 진단, 로그 분석, 명령 실행
- **표준 REST API** — `/api/v1`(브라우저 UI) 및 `/agent/v1`(머신 간)
- **Docker 관리** — 컨테이너, 이미지, 볼륨, 네트워크
- **웹 터미널** — WebSocket + pty 기반 SSH 유사 대화형 셸
- **7개 언어 i18n** — 간체/번체 중국어, 영어, 일본어, 한국어, 태국어, 독일어
- **단일 바이너리** — 프런트엔드를 `go:embed`로 내장, 런타임 의존성 없음

### 기술 스택

Go · Gin · GORM · SQLite(순수 Go) · Vue 3 · ECharts

### 빠른 시작

```bash
make build          # 현재 플랫폼용 빌드
make build-all      # Linux amd64/arm64 크로스 컴파일

./dist/opsmini -config configs/config.yaml
# http://localhost:8888 열기 (기본 계정: opsmini, 비밀번호는 시작 로그에서 확인)
```

### 문서

- [개발자 가이드](docs/developer-guide.md)
- [백엔드 아키텍처](docs/backend-architecture.md)
- [빌드 및 배포](docs/build-and-deploy.md)
- [호스트 보안 설계](host-security-design.md)
- [보안 감사 보고서](security-audit.md)

### 저작권

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
