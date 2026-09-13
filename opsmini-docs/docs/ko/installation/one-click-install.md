# 원클릭 스크립트 설치

공식 `install.sh`로 Linux 호스트에서 원클릭 배포를 완료합니다.

## 빠른 시작

```bash
# 원클릭 설치（권장, 스크립트가 아키텍처에 따라 알리바바 클라우드 OSS에서 바이너리 자동 다운로드）
curl -fsSL https://opsmini.com/install.sh | sudo bash

# 로컬 바이너리 지정 설치（기본 /data/opsmini에 설치, 포트 8888）
sudo ./install.sh -b ./opsmini

# 다운로드 주소 사용자 지정（순수 바이너리 또는 .tar.gz 지원）
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# 디렉터리와 포트 사용자 지정
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## 파라미터

| 파라미터 | 설명 | 기본값 |
|------|------|--------|
| `-d, --dir <path>` | 설치 디렉터리 | `/data/opsmini` |
| `-p, --port <port>` | 패널 리스닝 포트 | `8888` |
| `-b, --binary <path>` | opsmini 바이너리 경로 | 미지정 시 OSS에서 다운로드 |
| `-u, --url <url>` | URL에서 다운로드（순수 바이너리 또는 `.tar.gz`） | OSS 공식 주소 |
| `-n, --no-systemd` | systemd 서비스 미등록 | — |
| `-h, --help` | 도움말 | — |

## 설치 동작

1. 설치 디렉터리 생성（기본 `/data/opsmini`）
2. 바이너리를 `<디렉터리>/opsmini`로 다운로드 / 복사（기본적으로 아키텍처에 따라 알리바바 클라우드 OSS에서 다운로드）
3. `<디렉터리>/config.yaml` 생성（포트, SQLite 경로, JWT 유효 기간, 로그 레벨과 로그 파일; JWT 키와 Agent Token은 최초 시작 시 자동 생성되어 DB에 저장）
4. 16자리 랜덤 관리자 비밀번호를 생성하여 `<디렉터리>/.init_passwd`에 기록（권한 600）
   - **신규 설치**: 최초 시작 시 `OPSMINI_INIT_PASSWORD` 환경 변수로 주입
   - **재설치**（데이터베이스가 이미 존재）: 자동으로 `-reset-pass`로 비밀번호를 초기화하며, 출력된 비밀번호가 곧 유효 비밀번호입니다
5. systemd 서비스 `opsmini.service` 등록 및 시작
6. 서비스 응답을 탐지하고 접근 주소 / 사용자 이름 / 비밀번호 / 로그 파일 경로 출력

## 산출물

```
/data/opsmini/
├── opsmini              # 바이너리
├── config.yaml          # 설정（600）
├── opsmini.db           # SQLite 데이터베이스（최초 시작 후 생성）
├── opsmini.log          # 실행 로그 파일
└── .init_passwd         # 초기 비밀번호（600）
```

## 바이너리 확보

- **공식 배포（알리바바 클라우드 OSS）**: `https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v<버전>-linux-<아키텍처>`, 스크립트가 기본적으로 이곳에서 다운로드합니다
- **로컬 개발**: 프로젝트 루트의 `make build-all`이 `dist/opsmini-<ver>-linux-amd64` / `-arm64`를 산출하며, 스크립트가 아키텍처에 따라 자동으로 찾습니다

## 호환성

- `x86_64` / `aarch64`만 지원
- systemd가 없는 환경에서는 `-n`으로 서비스 등록을 건너뛰고 수동 시작을 사용합니다

## 계정 복구 {: #account-recovery }

비밀번호를 잊거나 MFA 인증 코드를 분실하면 서버에서 실행합니다（먼저 서비스를 중지하고, 작업 완료 후 다시 시작）:

```bash
# 비밀번호 초기화
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-pass opsmini
systemctl start opsmini

# MFA 바인딩 해제
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-mfa opsmini
systemctl start opsmini
```

> 설명: `-reset-mfa` / `-reset-pass`는 계정 복구 서브 명령으로, SQLite 데이터베이스를 직접 조작한 후 종료하며 Web 서비스를 시작하지 않습니다. 반드시 서비스가 중지된 상태에서 조작해야 합니다.
