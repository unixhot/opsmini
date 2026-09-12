# 데이터 저장

OpsMini는 임베디드 SQLite를 사용하여 모든 비즈니스 데이터（사용자, 작업, 알림, 웹사이트, 감사 로그 등）를 저장합니다.

## 데이터 파일

- 기본 경로: 원클릭 설치는 `/data/opsmini/opsmini.db`（`config.yaml`의 `database.path`로 지정, 기본적으로 설정 파일과 같은 디렉터리 기준 상대 경로）
- 최초 시작 시 자동 생성 및 테이블 생성（GORM 자동 마이그레이션）
- 민감 필드（키 / Token / TOTP secret）는 AES-GCM으로 암호화되어 저장됩니다

## 백업

SQLite는 단일 파일이므로 직접 복사하면 백업됩니다:

```bash
# 일관성 보장을 위해 먼저 서비스를 중지한 뒤 백업하는 것을 권장합니다
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /backup/opsmini.db.$(date +%s)
sudo systemctl start opsmini
```

## 복구

```bash
sudo systemctl stop opsmini
cp /backup/opsmini.db.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```

## 저장 내용 개요

| 데이터 | 테이블 | 설명 |
|------|-----|------|
| 사용자와 역할 | `users` / `roles` | 계정, 비밀번호 해시, RBAC 역할과 권한 |
| 세션 | `sessions` | refresh token과 유효 기간 |
| 패널 설정 | `settings` | 테마 / 언어 / 메뉴 표시·숨김 등 KV |
| 웹사이트 / 데이터베이스 | `websites` / `databases` | 사이트와 데이터베이스 인스턴스 기록 |
| 예약 작업 | `cron_jobs` | 패널 예약 작업 |
| 알림 규칙 / 이벤트 | `alert_rules` / `alert_events` | 모니터링 알림 |
| 감사 / 액세스 로그 | `audit_logs` / `access_logs` | 작업과 액세스 기록（기본 7일 보관） |

## 데이터 정리

- 감사 로그와 액세스 로그는 기본 7일 보관되며 매시간 자동 정리되고, 패널 설정에서 보관 일수를 조정할 수 있습니다
- 알림 이벤트는 보관 기간에 따라 자동 정리됩니다（기본 30일）
