# 최초 로그인과 초기화

설치를 완료한 후 다음 단계에 따라 최초 로그인과 보안 초기화를 진행합니다.

## 1. 초기 비밀번호 확인

시작 로그에 초기 계정 정보가 출력됩니다:

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

비밀번호는 설치 디렉터리의 `/data/opsmini/.init_passwd`（권한 600）에서도 확인할 수 있습니다.

## 2. 패널 로그인

1. 브라우저에서 `http://<host>:8888`을 엽니다
2. 사용자 이름 `opsmini`와 초기 비밀번호를 입력합니다
3. 로그인 성공 후 대시보드로 진입합니다

> ⚠️ 프로덕션 환경에서는 반드시 [리버스 프록시 + HTTPS](../configuration/https.md)를 통해 패널을 노출하여 평문 전송을 피하세요.

## 3. 비밀번호 변경

1. 「개인 설정」으로 들어갑니다
2. 「비밀번호 변경」에서 새 비밀번호를 설정하고 저장합니다

비밀번호를 잊은 경우 서버에 로그인하여 `opsmini -reset-pass <username>`를 실행해 초기화할 수 있습니다.

## 4. 2단계 인증 바인딩（권장）

1. 패널「설정 → 2단계 인증」에서 전역 2FA를 켭니다
2. 개인 보안 설정으로 들어가 TOTP 인증기를 스캔하여 바인딩합니다（Google Authenticator / 1Password 등）
3. 동적 코드를 입력하여 바인딩을 완료합니다

바인딩 후 매번 로그인 시 6자리 동적 코드를 추가로 입력해야 하며, 계정 보안이 크게 향상됩니다. 인증기를 분실하면 `opsmini -reset-mfa <username>`로 복구할 수 있습니다.

## 5. 보안 진입 설정（선택 사항）

패널「설정 → 기본 설정」에서 보안 진입 프리픽스를 설정합니다（또는 `config.yaml`의 `server.secret_entry`를 직접 수정）:

```yaml
server:
  secret_entry: "/opsmini_panel"
```

설정 후 패널 주소는 `http://<host>:8888/opsmini_panel`로 변경되며, 리버스 프록시와 함께 사용하면 실제 진입점을 숨겨 포트 스캔을 방지할 수 있습니다.

## 6. 다음 단계

- [AI 어시스턴트 설정](../configuration/panel-settings.md#ai-config)
- [대외 REST API 활성화](../api/agent-api.md)
- [기능 탐색](../features/index.md)
