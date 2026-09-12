# 자주 묻는 질문

## 설치와 시작

**Q: `go mod tidy`가 멈추거나 Bad Gateway 오류가 발생하나요?**

중국 내 네트워크에서 `proxy.golang.org`가 차단될 수 있으므로 `export GOPROXY=https://goproxy.cn,direct`를 실행하세요.

**Q: Go 버전이 너무 낮다는 메시지가 나오나요?**

OpsMini는 Go 1.25+가 필요합니다. 공식 사전 컴파일 바이너리를 다운로드하고 PATH에 추가하세요.

**Q: `/`에 접속하면 301 또는 빈 페이지가 나오나요?**

프런트엔드 정적 리소스는 이미 `go:embed`로 임베드되어 있으므로 `make build` 산출물을 사용하세요. 프런트엔드 디렉터리를 수동으로 분리하지 마세요.

## 계정과 보안

**Q: 관리자 비밀번호를 잊었나요?**

서버에서 `opsmini -config <path> -reset-pass opsmini`를 실행하여 랜덤 비밀번호로 초기화하세요（자세한 내용은 [설치 배포](../installation/index.md) 참조）.

**Q: MFA 2단계 인증 코드를 분실했나요?**

`opsmini -config <path> -reset-mfa opsmini`를 실행하여 MFA 바인딩을 해제한 뒤 다시 로그인하여 바인딩하세요.

## 배포와 운영

**Q: 버전을 어떻게 업그레이드하나요?**

`opsmini.db` 백업 → 바이너리 교체 → 서비스 재시작. SQLite는 GORM이 자동 마이그레이션하므로 일반적으로 수동 테이블 변경이 필요 없습니다.

**Q: 웹 터미널이 리버스 프록시 뒤에서 연결되지 않나요?**

Nginx에서 WebSocket 업그레이드 헤더를 활성화해야 합니다（`proxy_http_version 1.1` + `Upgrade`/`Connection`）, 자세한 내용은 [포트와 리버스 프록시](../configuration/ports-proxy.md)를 참조하세요.

**Q: Prometheus로 로컬 머신을 모니터링하려면?**

설정에서 `metrics`를 활성화하면 Prometheus가 `http://<host>:8888/metrics`를 직접 scrape합니다.
