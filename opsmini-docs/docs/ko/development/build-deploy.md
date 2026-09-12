# 빌드와 배포

## 사전 의존성

- Go 1.25+（순수 Go 드라이버로 CGO가 없어 직접 크로스 컴파일 가능）

## 코드 가져오기

```bash
git clone <저장소 주소> opsmini
cd opsmini
export GOPROXY=https://goproxy.cn,direct   # 중국 내 가속
go mod tidy
```

## 빌드 명령

```bash
make build          # 현재 플랫폼 컴파일 → dist/opsmini
make build-all      # linux/darwin amd64/arm64 크로스 컴파일
make clean          # dist/ 정리
make version        # 버전 정보 출력
```

## 버전 번호 주입

빌드 정보는 `-ldflags -X main.version/-X main.buildTime/-X main.gitCommit`을 통해 주입됩니다:

```bash
make build VERSION=v1.0.0
./dist/opsmini -version
# opsmini v1.0.0 (commit abc1234, built 2026-08-13T07:00:00Z)
```

## 산출물 명명

`opsmini-<version>-<os>-<arch>`, 예: `opsmini-v1.0.0-linux-amd64`.

## 제공 산출물

| 산출물 | 설명 |
|------|------|
| `opsmini` 단일 바이너리 | 백엔드 API + 프런트엔드 UI + SQLite, 약 28~30MB 정적 링크 |
| `configs/config.yaml` | 설정 템플릿 |
| `opsmini.db` | 최초 실행 시 자동 생성 |

> 배포는 바이너리 + 설정 파일을 복사하는 것뿐이며 다른 런타임 의존성이 없습니다.
