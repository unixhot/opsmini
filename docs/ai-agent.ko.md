<div align="center">

# OpsMini AI 에이전트 설계 (AI Agent)

**AI Agent Design**

[English](ai-agent.md) · [简体中文](ai-agent.zh-CN.md) · [繁體中文](ai-agent.zh-TW.md) · [日本語](ai-agent.ja.md) · [한국어](ai-agent.ko.md) · [ไทย](ai-agent.th.md) · [Deutsch](ai-agent.de.md)

</div>

---

> 마일스톤 M8~M10 ｜ 코드: `internal/aiagent/`
>
> 사용법: AI 어시스턴트 → 모델 설정 → 자연어로 바로 지시

M8에서 OpsMini의 AI 어시스턴트는 "실시간 시스템 스냅숏을 읽을 수 있는 운영 자문"에서 **실제로 호스트를 조작할 수 있는 에이전트**로 발전했습니다. 모델이 스스로 계획하고, 도구를 호출하고, 결과를 관찰하며 결론에 이를 때까지 반복합니다. 동시에 고위험 작업은 지금도 반드시 사람의 명시적 승인을 거쳐야 합니다.

## 1. Agent 프레임워크 대신 자체 경량 커널을 택한 이유

Go에는 성숙한 에이전트 프레임워크가 있습니다(Google ADK Go, CloudWeGo Eino). 그럼에도 OpsMini가 약 1,500줄의 커널을 직접 구현하는 이유는 다음과 같습니다.

1. **해자는 오케스트레이션이 아니라 도메인 능력에 있습니다.** 프레임워크가 해결하는 것은 범용적인 "모델 + 도구" 루프입니다. OpsMini의 가치는 29개의 도메인 도구, 단계별 안전 가드레일, gRPC 다중 호스트 경로에 있으며 이는 프레임워크가 제공하지 않습니다.
2. **접착 코드의 양이 커널과 비슷합니다.** 도구는 모두 평범한 로컬 Go 함수입니다. 이를 프레임워크의 도구 인터페이스에 맞추는 작업은 자체 구현과 비슷한 수고입니다.
3. **단일 바이너리, CGO 없음.** ADK를 넣으면 OpenTelemetry／genai／GCS가 의존성 트리에 함께 들어옵니다.
4. **확장 지점은 프레임워크가 아니라 프로토콜로 둡니다.** 도구는 **MCP 호환 JSON Schema**로 선언하며 프레임워크 전용 형식은 쓰지 않습니다. 덤으로 같은 레지스트리를 그대로 MCP 서버로 공개할 수 있습니다.

따라서 선택한 것은 **프로토콜화된 도구 + 자체 커널**입니다. 나중에 다중 에이전트 협업이나 시각적 워크플로 오케스트레이션이 필요해지면 Eino ADK나 Google ADK Go로 전환하고 MCP／A2A로 OpsMini와 상호 운용할 수 있습니다.

## 2. 커널 구조

```
internal/aiagent/
├── types.go      Tool / Result / Message / ToolCall / RiskLevel
├── registry.go   스레드 안전 레지스트리 (OpenAI 및 MCP 정의)
├── llm.go        OpenAI 호환 클라이언트 (스트리밍 tool-call 조각 누적)
├── loop.go       ReAct 커널: 추론 → 실행 → 관찰, 중단/재개 지원
├── guard.go      L3 하드 금지 패턴, 패널 자기 보호, 위험→정책 매핑
├── hitl.go       영속화되고 만료되는 사람 승인
├── memory.go     대화 영속화와 컨텍스트 압축
├── mcpclient.go  MCP 클라이언트 (공식 Go SDK, stdio + Streamable HTTP)
├── mcpserver.go  MCP 서버 (도구 집합을 외부 클라이언트에 공개)
├── events.go     SSE 이벤트 모델
└── tools/        내장 도구 집합
```

## 3. ReAct 루프

사용자 한 턴이 한 번의 `Agent.Send`에 대응합니다.

1. SQLite에서 대화 기록을 읽어 시스템 프롬프트와 실시간 호스트 스냅숏을 앞에 붙입니다.
2. 모델을 **스트리밍**으로 호출합니다. 텍스트 델타는 `thinking` 이벤트로 프런트엔드에 전달됩니다.
3. 모델이 도구를 요청하면:
   - 먼저 **L3 하드 금지 스캔**을 수행합니다. 적중하면 거부하고 이유를 모델에 되돌려 줍니다.
   - 다음으로 위험 등급과 관리자가 설정한 도구별 정책에 따라 즉시 실행／승인 필요／비활성 중 하나를 적용합니다.
   - 관찰 결과를 `tool` 메시지로 되돌려 주고 다음 턴으로 진행합니다.
4. 모델이 도구 요청을 멈추면 최종 답변을 출력합니다.

턴당 단계 상한은 기본 12입니다. 초과하면 도구 호출을 멈추고 지금까지 파악한 내용을 보고하므로 폭주하지 않습니다.

## 4. 위험 등급과 가드레일

| 등급 | 이름 | 동작 |
|------|------|------|
| L0 | `RiskReadonly` | 읽기 전용, 무인 실행 |
| L1 | `RiskLowWrite` | 저위험 쓰기. **기본값은 여전히 확인 필요**이며, 관리자가 특정 도구만 무인 실행으로 완화할 수 있음 |
| L2 | `RiskHighWrite` | 고위험. **매 실행마다 사람의 승인 필요**, 자동 실행으로는 절대 설정 불가 |
| L3 | 패턴 기반 | 실행 전에 차단. 승인이 있어도 실행하지 않음 |

L3가 다루는 것은 `rm -rf /`(핵심 시스템 디렉터리 포함), `mkfs`, `dd of=/dev/sd*`, 블록 디바이스로의 리다이렉트, 종료/재부팅, `chmod -R 777 /`, `chown -R … /`, 포크 폭탄, iptables／nft 규칙 초기화, `/etc/passwd`류 인증 파일 덮어쓰기, 셸 히스토리 삭제, `rmmod`, `kill -9 1`입니다.

패널 자체를 지키는 두 가지 규칙이 더 있습니다.

- 패널 데이터 디렉터리(바이너리, SQLite 데이터베이스, 스킬)를 삭제하거나 덮어쓰는 작업은 거부됩니다.
- 패널의 수신 포트를 끊는 방화벽 규칙은 거부됩니다.

> 제품 판단: 무인 root 에이전트는 만들지 않습니다. 목표 형태는 **"자율 계획 + 단계별 권한 위임"** 입니다. AI는 조사하고 조언하며, 파괴적인 작업은 사람이 승인합니다.

## 5. 휴먼 인 더 루프

승인이 필요한 호출이 발생하면 커널은 `ai_pending_approvals` 레코드(세션, 도구, 인수, 위험 등급, 확인 문구, 만료 시각 — 기본 10분)를 기록하고 `confirm` SSE 이벤트를 보낸 뒤 **턴을 종료**합니다. 프런트엔드가 확인 카드를 표시합니다.

사용자가 승인하거나 거부하면 `POST /ai/agent/approve`가 데이터베이스에서 메시지 시퀀스를 복원해 재개합니다.

- 도구 결과는 항상 호출 순서대로 영속화되며 미확인 호출에서 멈춥니다. 따라서 **중단된 호출의 인덱스는 그 assistant 메시지 바로 뒤에 오는 tool 메시지 개수와 같습니다**. 재개에 별도 기록이 필요 없습니다.
- 같은 턴에서 그 뒤에 대기하던 호출에는 명시적인 "미실행" 결과가 기록되어 모델 입장에서 로그 정합성이 유지됩니다.

## 6. 도구 목록 (29개)

| 분류 | 개수 | 예 |
|------|------|----|
| L0 읽기 전용 | 16 | `sys_overview`, `list_processes`, `list_ports`, `dir_usage`, `read_file`, `search_logs`, `query_metrics`, `docker_ps`, `security_status` |
| L1 저위험 쓰기 | 5 | `clean_temp`, `clean_logs`, `service_control`, `docker_control`, `install_skill` |
| L2 고위험 쓰기 | 8 | `exec_command`, `write_file`, `delete_path`, `modify_sshd`, `compress_dir`, `cron_write`, `kill_process`, `firewall_rule` |
| 외부 MCP | 동적 | `mcp__<server>__<tool>`, 위험 등급은 `readOnlyHint` 주석에서 도출 |

도구는 패널의 RBAC를 그대로 따릅니다. 읽기 전용 역할은 쓰기 권한이 필요한 도구를 아예 보지 못하고, L2 도구는 추가로 `ai.agent.write` 권한이 필요합니다.

## 7. 양방향 MCP

### MCP 클라이언트로서 (외부 도구 사용)

"AI 어시스턴트 → MCP"에서 MCP 서버를 설정합니다(stdio 명령 또는 사용자 헤더가 있는 Streamable HTTP URL). 커널은 공식 MCP Go SDK로 접속해 도구를 나열하고 각각을 `mcp__<server>__<tool>`로 등록합니다. 연결은 2분마다 대조하므로 설정을 바꿔도 재시작이 필요 없습니다.

### MCP 서버로서 (능력 공개)

패널은 자체 도구 집합을 `/mcp`(Streamable HTTP)로 공개합니다.

- 인증은 **베어러 토큰**을 사용하며 최초 기동 시 생성됩니다. "AI 어시스턴트 → 도구 권한"에서 확인／복사／재발급할 수 있습니다. MCP 클라이언트는 패널의 JWT 로그인을 수행할 수 없으므로 전용 토큰을 둡니다.
- 기본적으로 **L0／L1 도구만** 공개됩니다. 외부 MCP 클라이언트는 패널의 확인 대화상자를 띄울 수 없으므로, L2 도구는 "MCP 고위험 작업 허용"을 켜기 전까지 전달되지 않습니다.
- L3 가드레일과 패널 자기 보호는 원격 호출에도 적용됩니다.
- 모든 원격 호출은 수행자 `mcp-client`로 감사 로그에 기록됩니다.

클라이언트 설정 예시(Claude Desktop／Cursor 등 Streamable HTTP 지원 클라이언트):

```json
{
  "mcpServers": {
    "opsmini": {
      "type": "http",
      "url": "https://your-panel.example.com/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

## 8. 세션과 기억

- `ai_sessions` / `ai_messages`는 SQLite에 저장되며 `tool_calls`와 `tool_call_id`를 기록합니다. 페이지를 새로 고치거나 프로세스를 재시작해도 사고 사슬을 재개하거나 재생할 수 있습니다.
- 기록이 임계값을 넘으면(최근 24건 원문 유지 + 12건 버퍼) **압축**됩니다. 그보다 오래된 내용은 모델이 300자 이내 요약으로 만들고 하나의 system 메시지로 되돌려 넣습니다.
- 세션은 기본 30일간 보관되며(`ai_session_retention_days`) 백그라운드에서 정리됩니다.

## 9. API 목록

| 메서드 | 경로 | 용도 |
|--------|------|------|
| POST | `/api/v1/ai/agent/chat` | 스트리밍 대화(사고 사슬／도구 호출／확인) |
| POST | `/api/v1/ai/agent/approve` | 대기 중인 호출을 승인/거부하고 재개 |
| GET | `/api/v1/ai/agent/tools` | 도구 목록(위험, 권한, 현재 정책)과 MCP 상태 |
| PUT | `/api/v1/ai/agent/tools/policy` | 도구별 정책(자동／확인／비활성) |
| GET | `/api/v1/ai/agent/sessions` | 세션 목록 |
| GET | `/api/v1/ai/agent/sessions/:id/messages` | 대화 기록(대기 중 승인 포함) |
| DELETE | `/api/v1/ai/agent/sessions/:id` | 세션 삭제 |
| GET | `/api/v1/ai/agent/config` | 런타임 설정과 모델 상태 |
| GET | `/api/v1/ai/agent/sessions/:id/report` | 세션 보고서 내보내기(기본 HTML) |
| GET | `/api/v1/ai/agent/reports` | 보관된 보고서 목록(형식 + 보관 디렉터리 포함) |
| GET | `/api/v1/ai/agent/reports/view?name=` | 보관된 보고서를 **바로 열기**(inline, `?token=` 허용) |
| GET | `/api/v1/ai/agent/reports/download?name=` | 보관된 보고서 다운로드(attachment, `?token=` 허용) |
| GET/PUT | `/api/v1/ai/agent/mcp-server` | 인바운드 MCP 엔드포인트 설정(활성화／토큰／고위험) |
| ANY | `/mcp` | 인바운드 MCP 엔드포인트(베어러 토큰) |

권한: `ai.agent`(에이전트 사용)와 `ai.agent.write`(고위험 작업 승인). 이전 엔드포인트 `/ai/chat`, `/ai/chat/stream`은 폴백 경로로 남아 있습니다.

## 10. 다중 호스트 기능의 소속: 패널은 단일 호스트만

OpsMini 는 **단일 호스트 패널**이며, AI 도구 집합은 이 머신만 조작합니다. 여러 호스트 조작은 OpsAnt 의 역할입니다. OpsAnt 가 중심이며 salt / ansible / opsmini 세 가지 실행 엔진과 통합 실행 API(`/api/opsant/v1/tasks/run-shell/`, `tasks/run-script/`, `tasks/send-file/`, `tasks/get-minion-file/` 등)를 보유합니다.

초기 버전에는 4개의 다중 호스트 도구(`remote_list_hosts`, `remote_exec`, `remote_pull_file`, `remote_push_file`)가 있었고, OpsMini Server 의 컨트롤 플레인 HTTP API(4508)를 통해 다른 호스트로 작업을 보냈습니다. 이 도구들은 아키텍처 경계를 넘었기 때문에 제거했습니다.

- 4508 은 OpsMini Server 가 OpsAnt(Django)에게 제공하는 **내부** API 로, "로컬/내부망 전용"이며 인증을 검사하지 않습니다.
- 이 경로에서 패널의 역할은 **관리 대상 Agent** 이며, 4507 의 gRPC 장기 연결로 Server 와 통신(등록·하트비트·작업 수신)해야 하고, 컨트롤 플레인을 역으로 호출해서는 안 됩니다.
- 그 잘못된 설계를 동작시키려면 4508 을 외부에 노출해야 하며, 이는 인증 없는 원격 명령 실행 입구를 인터넷에 두는 것과 같습니다.

따라서 역할 분담은 **단일 호스트 운영과 AI 는 OpsMini, 다중 호스트 오케스트레이션과 AI 는 OpsAnt** 입니다. 둘은 "OpsMini 를 OpsAnt 의 한 가지 관리 방식으로 취급"하는 형태로 연결되며, 데이터 흐름은 항상 OpsAnt → Agent 입니다.

## 11. 예약 AI 작업(호스트 점검 등)

"AI 작업"은 새로운 진입점이 아니라 **예약 작업의 네 번째 종류**(`kind = agent`)입니다. "패널 작업／시스템 crontab／사용자 crontab"과 나란히 놓입니다. 따라서 스케줄링, 활성화/비활성화, 마지막 실행 시각, 실패 알림이 모두 기존 메커니즘을 재사용하며, "매일 아침 무언가 실행"은 사용자가 이미 보는 위치에 자리합니다.

### 실행 파이프라인

```
cron 발화 -> 에이전트가 무인 실행(ReAct, 읽기 전용／저위험 도구)
          -> 대화 저장(보고서가 곧 AI 세션이며 사고 사슬을 포함)
          -> HTML과 Markdown을 <데이터 디렉터리>/reports/<작업 이름>/에 보관
          -> 요약을 작업에 설정된 알림 채널로 전송
```

### 무인 실행에서는 확인 정책을 거부 쪽으로 완화

예약 실행에는 "승인"을 누를 사람이 없습니다. 따라서 `RunOptions.Unattended = true`이면:

- `PolicyConfirm`(L2)에 해당하는 도구는 **즉시 거부**됩니다. 이유가 모델에 전달되고("읽기 전용 도구로 정보를 수집하고, 이 단계에는 사람이 필요하다고 결론에 명시하세요"), 거부는 세션의 `Skipped` 목록에 기록됩니다.
- 모델은 읽기 전용 도구로 전환하고 작업은 끝까지 완료됩니다. 보고서에는 사람이 실행해야 할 작업과 그대로 복사할 수 있는 명령이 적힙니다.
- L0 읽기 전용과 L1 저위험 도구는 영향을 받지 않습니다. 그래서 "매일 점검"은 구조적으로 읽기 전용입니다. 범위를 넓히려면 정책을 명시적으로 바꿔야 합니다.

거부된 항목은 `cron_jobs.skipped_tools`에 남고 알림과 보고서 부록 양쪽에 나타납니다.

### 신뢰성과 비용 제어

| 장치 | 설명 |
|------|------|
| 단계 상한 | 작업별 `max_steps`(기본 12, 폼에서 수정 가능) |
| 전체 시간 상한 | 작업별 `timeout_seconds`(기본 600초). 시간 초과는 실패로 처리 |
| 연속 실패 시 자동 비활성화 | 3회 연속 실패하면 작업을 자동 비활성화해 "매일 실패하고 매일 알림"을 막음 |
| 수동 저장으로 초기화 | 패널에서 작업을 저장하면 `fail_count`가 0으로 돌아감 |

### 보고서: 단일 HTML 파일

실행 한 번에 **자체 완결형 HTML 파일 하나**를 만듭니다. 이것이 그대로 열거나 내려받는 산출물입니다.

```
<데이터 디렉터리>/reports/<작업 이름>/<작업 이름>-<타임스탬프>.html   <- 보고서 본체(자체 완결)
<데이터 디렉터리>/reports/<작업 이름>/<작업 이름>-<타임스탬프>.md     <- 같은 내용의 Markdown
```

`<데이터 디렉터리>`는 패널 설정의 데이터 디렉터리(`appstore_data_dir`, 기본 `/var/lib/opsmini/data`)입니다. **생성물은 모두 그 아래에 기능별 하위 디렉터리로 둡니다.** 보고서는 `reports/`, 앞으로의 생성 기능도 같은 위치에 디렉터리를 더하면 됩니다.

"자체 완결"이란 스타일이 인라인이고 스크립트, 외부 스타일시트, 웹 폰트, 이미지가 없다는 뜻입니다. 그래서 USB 메모리에 담아도, 메일 첨부로 보내도, 아무 정적 호스팅에 올려도 제대로 표시됩니다. 패널은 **일부러 CJK 폰트를 내장하지 않습니다**(내장하면 단일 바이너리가 10MB 이상 커집니다). 보고서는 읽는 사람의 시스템 폰트를 사용하며 데스크톱과 모바일 브라우저 모두 기본으로 갖추고 있습니다.

내용은 실행 요약(상태／단계 수／도구 호출 수), 작업 지시, 결론, 그리고 **부록: 실행 로그**입니다. 부록은 도구 호출마다 한 행이며 인수와 관찰 결과를 나란히 두고 상태는 "완료／거부／하드 금지／실패" 중 하나입니다.

작업 목록의 각 행에는 두 개의 진입점이 있습니다.

- **열기** — `GET /ai/agent/reports/view`, `Content-Disposition: inline`. 새 브라우저 탭에서 렌더링됩니다.
- **다운로드** — `GET /ai/agent/reports/download`, `Content-Disposition: attachment`.

본문은 모델이 쓴 Markdown에 호스트 데이터(로그 발췌, 프로세스 명령줄, 설정 값)가 섞인 것입니다. 렌더러는 **문서 전체를 먼저 이스케이프한 뒤** 인라인 규칙을 적용하고 `javascript:` 같은 대상은 링크로 만들지 않습니다. 따라서 로그 줄에 `<script>`가 있어도 텍스트로 표시됩니다. 여기에 더해 엄격한 CSP를 응답 헤더로 보내 이중 방어를 합니다.

PDF가 필요하면 열린 보고서에서 브라우저의 "인쇄 → PDF로 저장"을 사용하십시오. 스타일시트에 인쇄 규칙이 있어 화면과 같은 결과가 나옵니다.

### 임시 대화도 내보내기 가능

AI 어시스턴트의 어떤 대화든 같은 HTML 보고서로 내보낼 수 있습니다. `GET /ai/agent/sessions/:id/report`(기본 HTML, `?format=md`면 Markdown, `?download=1`이면 강제 다운로드). 디스크에 보관되지 않은 세션도 읽을 수 있는 보고서를 얻습니다.

### 알림 경로 공유

AI 작업과 알림은 하나의 `Notifier`(`internal/service/notify.go`)를 공유하며 패널 알림, 이메일, WeCom／DingTalk／Feishu 그룹 로봇을 지원합니다. 채널 목록이 비어 있으면 패널 알림만 발송됩니다.
