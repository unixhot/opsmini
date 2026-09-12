# 사용자와 권한

OpsMini는 역할 기반 액세스 제어（RBAC）를 채택하여 패널 작업 권한을 정밀하게 관리합니다.

## 사용자 관리

- 사용자 생성 / 편집 / 비활성화 / 삭제
- 마지막 로그인 시간 조회
- 각 사용자는 독립적인 MFA（TOTP）바인딩 가능
- 개인 설정 페이지에서 닉네임, 아바타, 이메일, 비밀번호 수정 지원

## 역할과 권한

내장된 세 가지 역할:

| 역할 | 권한 범위 |
|------|----------|
| `admin` | 전체 권한（사용자 / 역할 / 설정 포함） |
| `operator` | 일상 운영（앱, 컨테이너, 파일, 터미널, 예약 작업 등） |
| `readonly` | 읽기 전용 조회 |

사용자 지정 역할을 지원하며, **권한 포인트** 단위로 정밀하게 할당합니다, 예:

- `user.create` / `user.edit` / `user.delete`
- `website.create` / `website.edit` / `website.delete`
- `container.edit` / `container.delete`
- `security.scan` / `security.firewall` / `security.fim` / `security.threat`
- `settings.view` / `settings.edit`（패널 설정）
- `apps.install`、`cron.create`、`database.create`、`file.write`、`mcp.manage`、`skill.manage`、`alert.manage` 등

## 2단계 인증（2FA） {: #2fa }

- **전역 스위치**: 패널「설정 → 2단계 인증」을 켜면 인증기를 바인딩한 모든 계정이 로그인 시 6자리 동적 코드를 입력해야 합니다
- **사용자별 바인딩**: 사용자는 개인 보안 설정에서 QR코드를 생성하고, Google Authenticator / 1Password / 각 클라우드 제공자 인증기 APP으로 스캔하여 바인딩합니다
- **계정 복구**: 인증기를 분실하면 서버에 로그인하여 `opsmini -reset-mfa <username>`를 실행해 해당 사용자의 MFA 바인딩을 초기화할 수 있습니다

## 보안 설계

- 비밀번호 bcrypt 해시 저장
- 단기 access JWT（15분）+ 취소 가능한 refresh token（7일）
- JWT 서명 키는 최초 시작 시 자동 생성되어 데이터베이스에 영속화되며 설정 파일에 기록되지 않습니다
- 로그인 실패 속도 제한과 잠금（브루트포스 방지）
- 주요 작업은 감사 로그에 기록됩니다
- 비밀번호를 잊으면 서버에 로그인하여 `opsmini -reset-pass <username>`로 초기화할 수 있습니다
