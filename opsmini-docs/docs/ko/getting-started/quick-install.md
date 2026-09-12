# 5분 빠른 설치

공식 설치 스크립트 `install.sh`를 사용하여 Linux 호스트에서 원클릭으로 배포를 완료합니다.

## 사전 조건

- 대상 호스트: Linux `x86_64` 또는 `aarch64`
- `sudo` 권한 보유
- 외부 네트워크 접근 가능（바이너리 다운로드용）또는 바이너리 파일 사전 준비

## 원클릭 설치

```bash
# 원클릭 설치（권장, 스크립트가 아키텍처에 따라 알리바바 클라우드 OSS에서 바이너리 자동 다운로드）
curl -fsSL https://opsmini.com/install.sh | sudo bash

# 로컬 바이너리 지정 설치（기본 /data/opsmini에 설치, 포트 8888）
sudo ./install.sh -b ./opsmini

# 다운로드 주소 사용자 지정
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# 디렉터리와 포트 사용자 지정
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## 설치 파라미터

| 파라미터 | 설명 | 기본값 |
|------|------|--------|
| `-d, --dir <path>` | 설치 디렉터리 | `/data/opsmini` |
| `-p, --port <port>` | 패널 리스닝 포트 | `8888` |
| `-b, --binary <path>` | opsmini 바이너리 경로 | 미지정 시 OSS에서 다운로드 |
| `-u, --url <url>` | URL에서 다운로드（`.tar.gz` 또는 순수 바이너리） | OSS 공식 주소 |
| `-n, --no-systemd` | systemd 서비스 미등록 | — |

## 설치 동작

1. 설치 디렉터리 생성（기본 `/data/opsmini`）
2. 바이너리를 `<디렉터리>/opsmini`로 다운로드 / 복사（기본적으로 아키텍처에 따라 알리바바 클라우드 OSS에서 다운로드）
3. `<디렉터리>/config.yaml` 생성（포트, SQLite 경로, JWT 유효 기간, 로그 레벨과 로그 파일; JWT 키와 Agent Token은 최초 시작 시 자동 생성되어 DB에 저장）
4. 16자리 랜덤 관리자 비밀번호를 생성하여 `<디렉터리>/.init_passwd`에 기록（권한 600）
5. systemd 서비스 `opsmini.service` 등록 및 시작
6. 서비스 응답을 탐지하고 접근 주소 / 사용자 이름 / 비밀번호 / 로그 파일 경로 출력

## 산출물 구조

```
/data/opsmini/
├── opsmini              # 바이너리
├── config.yaml          # 설정（600）
├── opsmini.db           # SQLite 데이터베이스（최초 시작 후 생성）
├── opsmini.log          # 실행 로그 파일
└── .init_passwd         # 초기 비밀번호（600）
```

## 최초 로그인

- `http://<host>:8888` 접속
- 사용자 이름: `opsmini`
- 비밀번호: 설치 스크립트가 출력한 비밀번호, 또는 `/data/opsmini/.init_passwd` 확인

> ⚠️ 비밀번호를 즉시 기록하고 로그인 후 변경하세요. 재설치 시 비밀번호는 자동으로 초기화되어 다시 출력됩니다.

## 다음 단계

- [최초 로그인과 초기화](first-steps.md)
- [수동 배포（systemd）](../installation/manual-install.md)
- [리버스 프록시와 HTTPS](../configuration/ports-proxy.md)
