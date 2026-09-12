# 수동 설치（소스 코드 빌드）

원클릭 스크립트를 사용하지 않으려면 소스 코드에서 빌드하고 수동으로 배포할 수 있습니다.

## 사전 의존성

| 의존성 | 버전 요구 사항 | 설명 |
|------|----------|------|
| Go | **1.25+** | 순수 Go 드라이버로 CGO가 없어 임의 플랫폼에서 직접 크로스 컴파일 가능 |
| 메모리 | ≥ 512MB | 빌드와 실행 모두 경량 |

> **CGO 도구 체인이 필요 없는 이유**: SQLite 드라이버는 `github.com/glebarez/sqlite`（순수 Go 구현）를 사용하므로,
> `CGO_ENABLED=0`으로 임의 플랫폼에서 **정적 링크** 바이너리를 크로스 컴파일할 수 있습니다.

## 코드와 의존성 가져오기

```bash
git clone <저장소 주소> opsmini
cd opsmini

# 중국 내 네트워크는 goproxy 미러 설정 필요
export GOPROXY=https://goproxy.cn,direct

go mod tidy
```

## 빌드

```bash
# 현재 플랫폼 컴파일 → dist/opsmini
make build

# 전체 대상 플랫폼 크로스 컴파일
make build-all
# 산출물:
#   dist/opsmini-<version>-linux-amd64
#   dist/opsmini-<version>-linux-arm64
#   dist/opsmini-<version>-darwin-amd64
#   dist/opsmini-<version>-darwin-arm64

# 버전 번호 명시 지정
make build VERSION=v1.0.0

# 바이너리 버전 확인
./dist/opsmini -version
```

> 공식 릴리스 절차: `git tag v1.0.0 && make build-all`, `VERSION`은 자동으로 tag 이름을 가져옵니다.

## 산출물 아키텍처 검증

```bash
file dist/*
# 예상: ELF x86-64 / ELF aarch64 / Mach-O x86_64 / Mach-O arm64, 모두 정적 링크
```

## 수동 배포

### 디렉터리 계획

```
/data/opsmini/
├── opsmini              # 바이너리
├── config.yaml          # 설정 파일
└── opsmini.db           # 데이터（최초 시작 시 자동 생성）
```

### 바이너리와 설정 배치

```bash
sudo mkdir -p /data/opsmini
sudo cp dist/opsmini-*-linux-amd64 /data/opsmini/opsmini
sudo cp configs/config.yaml /data/opsmini/config.yaml

# 설정 수정: 데이터베이스 절대 경로（JWT 서명 키는 설정 불필요, 최초 시작 시 자동 생성）
sudo sed -i 's|path: "opsmini.db"|path: "/data/opsmini/opsmini.db"|' /data/opsmini/config.yaml

sudo chmod +x /data/opsmini/opsmini
```

### 실행

```bash
/data/opsmini/opsmini -config /data/opsmini/config.yaml
```

최초 시작 시 자동으로 테이블을 생성하고 기본 관리자 계정을 기록하며, 로그에 계정 비밀번호가 출력됩니다. `http://<host>:8888`에 접속하면 패널이 나타납니다.

## 다음 단계

- [systemd 호스팅](systemd.md)
- [설정 상세](../configuration/config-file.md)
