# Agent 연결(gRPC)

OpsMini 프로세스에는 **Agent 클라이언트**가 내장되어 있습니다. 스스로 외부로 gRPC 양방향 스트림 하나를 열어 등록·하트비트·작업 수신·파일 전송을 처리합니다. **상대편에 SSH도 인바운드 포트도 필요 없어** NAT, 사내망, 공인 IP 없는 호스트에 적합합니다.

!!! warning "이전 /agent/v1 REST API 안내"
    `/agent/v1` REST API 와 "설정 → API Token" 화면은 **v1.2.1 에서 제거**되었습니다. 외부 시스템에서 제어하려면 **MCP**(`/mcp`, [패널 API](browser-api.md) 참고)를, 여러 호스트 관리는 이 문서의 gRPC Agent 를 사용하세요.

## 통신 모델

| 방향 | |
|------|------|
| Agent → Server | `Register` · `Heartbeat` · `TaskResult` · `FileChunkAck` |
| Server → Agent | `RegisterAck` · `ExecuteTask` · `PushFileTask` · `Ping` |

- 포트 하나면 충분: HTTP/2 다중화로 SaltStack 같은 이중 포트가 필요 없음
- 연결이 끊기면 지수 백오프로 재연결하고 주기적 하트비트로 유지
- 같은 호스트 이름이 재접속하면 이전 연결은 정리됩니다

## 설정

```yaml
agent:
  server_addr: "47.104.172.52:4507"
  server_token: "<token from server>"
  heartbeat_seconds: 30
```

`server_addr` 를 비우면 Agent 모드가 꺼지고 패널은 단독으로 동작합니다.

## 지원 범위

| 기능 | 설명 |
|------|------|
| 명령 / 스크립트 실행 | `ExecuteTask` 의 oneof 로 command 와 script 구분 |
| 파일 배포 | `PushFileTask` 가 청크를 스트리밍하며 `resume_offset` 과 md5 검증 지원 |
| 파일 가져오기 | Agent 가 읽어서 그대로 반환 |
| 지표 수집 | gRPC 가 아니라 `/metrics` 로 Prometheus 가 수집 |

!!! note "서버 측 코드 위치"
    OpsMini Server 는 OpsAnt 프로젝트(`github.com/opsany/opsmini-server`)에 있으며, 이 저장소에는 Agent 측(`internal/agent/client.go`)만 포함됩니다.
