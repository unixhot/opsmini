# 로컬 개발 환경

## Go 1.25+ 설치

```bash
# macOS（Apple Silicon）
curl -sL -o /tmp/go.tar.gz https://go.dev/dl/go1.25.0.darwin-arm64.tar.gz
mkdir -p ~/go-sdk && tar -C ~/go-sdk -xzf /tmp/go.tar.gz
export PATH=~/go-sdk/go/bin:$PATH

# Linux x64
curl -sL -o /tmp/go.tar.gz https://go.dev/dl/go1.25.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf /tmp/go.tar.gz
export PATH=/usr/local/go/bin:$PATH
```

## 패널 실행

```bash
go run ./cmd/agent -config configs/config.yaml
# 또는 빌드 후 실행
make build && ./dist/opsmini -config configs/config.yaml
```

`http://localhost:8888`에 접속하며, 기본 계정 `opsmini`, 비밀번호는 시작 로그를 참조합니다.

## 프런트엔드 설명

- 프런트엔드는 단일 파일 `web/index.html`（Vue 3 인라인 SPA）+ `web/static/` 정적 라이브러리입니다
- `web/embed.go`의 `go:embed`를 통해 바이너리에 임베드됩니다
- 프런트엔드를 수정한 후 `make build`를 다시 실행해야 적용됩니다

## 자주 묻는 문제

- `go mod tidy`가 멈춤 → `GOPROXY=https://goproxy.cn,direct` 설정
- 빌드 시 `vendor` 디렉터리 충돌 오류 → 프런트엔드 의존성 디렉터리는 이미 `static/`으로 이름이 변경되었으므로 `vendor/`를 다시 만들지 마세요
