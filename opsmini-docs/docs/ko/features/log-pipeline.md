# 로그 파이프라인

OpsMini는 **Filebeat 설정과 호환되는** 로그 수집 실행 엔드포인트를 내장하고 있습니다. 호스트의 로그를 작업 설정에 따라 수집, 처리, Elasticsearch / Kafka / Logstash로 전송하며, 위치는 "관리 대상 호스트의 filebeat"에 해당하지만 별도의 프로세스를 설치할 필요가 없습니다.

## 기본적으로 비활성화됨

이 점은 먼저 명확히 해야 합니다: **기본 설치에서는 어떤 로그도 수집하지 않습니다**. goroutine도 없고, 디렉터리를 스캔하지 않으며, 메모리도 차지하지 않습니다.

세 가지 단계로 활성화되며, 어느 하나라도 충족되면 시작됩니다:

| 방식 | 트리거 조건 | 적용 |
|---|---|---|
| 작업 파일 | `tasks.d` 디렉터리에 임의의 `.yml` / `.yaml`가 나타남 | 수동으로 단일 호스트 수집 구성 |
| 명시적 스위치 | `config.yaml`의 `logpipeline.enabled: true` | 강제로 활성화하려는 경우 (당장 작업이 없더라도) |
| 플랫폼 배포 | OpsAnt가 배포한 `LogTask`를 수신하고 디스크에 기록 | 다중 호스트 중앙 관리 |

반대로, `enabled: false`이고 작업 파일이 없으면 파이프라인은 완전히 정지 상태입니다——이것이 "단일 호스트 자체 사용" 시나리오의 기본 상태입니다.

## 디렉터리 레이아웃

```
<데이터 디렉터리>/logs/
├── main.yml          # 메인 설정 (filebeat 형태: output + 전역 파라미터)
├── tasks.d/          # 수집 작업, 작업 하나당 파일 하나
│   ├── nginx.yml
│   └── app-1024.yml
├── registry.db       # 각 파일의 읽기 위치 (inode + offset)
└── spool/            # 출력 실패 시의 디스크 큐
```

`main.yml`는 "어디로 보낼지"를, `tasks.d/*.yml`는 "무엇을 수집할지"를 담당합니다. 이 분리는 filebeat의 `filebeat.yml` + `inputs.d/`와 일치하므로, 기존 filebeat 설정을 그대로 나누어 바로 사용할 수 있습니다.

## 작업 작성 방법

**방식 A(권장)**: 작업은 수집만 설명하고, 출력은 `main.yml`에서 통일적으로 결정합니다.

```yaml
# tasks.d/nginx.yml
- type: filestream
  id: nginx-access
  paths:
    - /var/log/nginx/access.log
  fields:
    service: nginx
    env: prod
  parsers:
    - ndjson:
        target: ""
        add_error_key: true
  processors:
    - drop_fields:
        fields: ["agent.ephemeral_id", "ecs.version"]
    - add_fields:
        target: ""
        fields:
          cluster: bj-prod
```

**방식 B**: 작업 파일에 `output`이 포함되어 있어 「서로 다른 작업을 서로 다른 곳으로 전송」하는 데 사용됩니다. 이는 filebeat 대비 OpsMini의 확장 기능입니다——filebeat는 여러 인스턴스가 있어야만 가능합니다.

```yaml
output.elasticsearch:
  hosts: ["http://10.0.0.5:9200"]
  index: "app-%{+yyyy.MM.dd}"
```

두 가지 방식 모두 허용되며, B의 우선순위가 `main.yml`보다 높습니다.

## 핫 리로드

`tasks.d` 10초마다 한 번씩 대조합니다:

- **추가** 작업 파일 → 파싱 후 수집을 시작합니다; 파싱 실패 시 실행 중인 이전 버전을 **덮어쓰지 않습니다**, 「필드 하나를 잘못 수정해 수집이 전부 중단되는」 상황을 방지합니다
- **수정** 작업 파일 → 내용 해시가 변경된 경우에만 재구성하며, 동일한 내용을 반복 전송해도 재시작이 트리거되지 않습니다 (재시작하면 multiline 버퍼가 유실되고 모든 파일이 다시 열립니다)
- **삭제** 작업 파일 → 해당 작업을 중지합니다

작업 파일 이름은 `*.yml` / `*.yaml`만 가능하며, 경로를 포함할 수 없고 점으로 시작할 수 없습니다——파일 이름은 원격에서 디렉터리를 탈출하는 데 사용될 수 있는 유일한 필드이므로, 아예 제한을 걸어둡니다.

## 지원되는 프로세서

일반적인 정제 요구 사항을 처리하는 31개의 Filebeat 프로세서:

| 카테고리 | 프로세서 |
|---|---|
| 필드 추가/삭제/수정 | `add_fields` `add_tags` `drop_fields` `include_fields` `rename` `copy_fields` `move_fields` `truncate_fields` |
| 타입 및 파싱 | `convert` `decode_json_fields` `decode_csv_fields` `decode_duration` `dissect` `syslog` `decode_xml` `decode_base64_field` `decompress_gzip_field` `urldecode_fields` `extract_array` |
| 조건 및 흐름 제어 | `drop_event` `script` |
| 시간 및 식별자 | `timestamp` `add_id` `fingerprint` |
| 메타데이터 | `add_host_metadata` `add_observer_metadata` `add_locale` `add_network_direction` |
| 네트워크 및 도메인 | `community_id` `registered_domain` |
| 텍스트 치환 | `replace` |

## 출력

| 유형 | 설명 |
|---|---|
| `elasticsearch` | index 템플릿 변수, 계정 비밀번호, 배치 크기 및 타임아웃 지원 |
| `kafka` | SASL 인증, topic 변수, 파티션 정책 지원 |
| `logstash` | Lumberjack v2 프로토콜, TLS 포함(`logstash` / `lumberjack` 두 가지 표기 모두 인식) |

출력이 구성되지 않은 경우 작업은 정상적으로 수집하지만 전송에 실패하며, 패널은 오류를 파이프라인 상태에 표시합니다. 조용히 폐기되지 않습니다.

## 데이터를 잃지 않는 방식

전달 의미론은 **at-least-once**입니다:

1. 로그 배치를 읽음 → 전달
2. **전달 성공 이후에만** `registry.db`의 offset을 진행
3. 전달 실패 → `spool/`에 기록(기본 상한 512MB) → 백오프 재시도
4. 프로세스 크래시 → 재시작 후 마지막으로 확인된 offset부터 재전송, 중복될 수 있지만 유실되지는 않음

거래 관점에서 로그 배치가 중복되는 것은 보통 무해하므로(ES 측에서는 `fingerprint` 프로세서로 멱등성을 처리할 수 있음), 로그 유실은 복구할 수 없기 때문에 전자를 선택했습니다.

## 실시간 미리보기

패널 **로그 → 파이프라인** 탭에서 확인할 수 있습니다:

- 파이프라인이 실행 중인지, 현재 출력 유형, 출력 오류
- 각 작업의 수집 파일 수, 읽기 속도, 마지막 이벤트 시간
- spool 적체량
- 구성 경고(알 수 없는 필드, 경로 없음 등)

작업을 선택하면 **최근 20개 이벤트를 실시간으로 미리 볼** 수 있어, 프로세서 체인을 거친 후 어떤 형태인지 확인할 수 있습니다. ES에서 먼저 조회할 필요도, 구성을 수정하고 재시작할 필요도 없습니다. 이것이 processor를 디버깅하는 가장 빠른 방법입니다.

## Filebeat 호환성이 필요한 이유

세 가지 이유:

1. **마이그레이션 비용**: 기존 ES 인덱스 템플릿, ILM 정책, Kibana 대시보드를 그대로 두고, `agent.type`를 `filebeat`로 설정해 바로 재사용할 수 있습니다
2. **운영 가독성**: 수집 설정은 운영자가 가장 익숙한 YAML 그대로이며, 새로운 DSL을 배울 필요가 없습니다
3. **새로운 의존성 미도입**: 기능을 단일 바이너리에 내장하여, 로그 수집을 위해 filebeat를 추가로 설치할 필요가 없습니다

## 하지 않는 부분

솔직하게 선을 그어, 오해를 피하기 위해:

- **module / autodiscover는 하지 않음** (Nginx 모듈, Kubernetes 자동 검색): 단일 바이너리 패널에는 컨테이너 오케스트레이션 컨텍스트가 없음
- **ILM과 인덱스 템플릿 관리는 하지 않음**: 그건 ES 측의 일임
- **호스트 간 집계는 하지 않음**: 다중 호스트 수집은 OpsAnt가 작업을 하달하여 통합 관리하고, OpsMini는 실행 측만 담당
- **Filebeat의 모든 출력을 지원하지 않음** (Logstash Beat, Redis, Kafka 외의 미들웨어): ES / Kafka / Logstash 세 가지만 지원

## 구성 참조

```yaml
# config.yaml
logpipeline:
  enabled: false            # 하드 스위치, true일 때 작업 파일이 없어도 시작
  tasks_dir: ""             # 기본값 <데이터 디렉터리>/logs/tasks.d
  config_file: ""           # 메인 구성, 기본값 <데이터 디렉터리>/logs/main.yml
  strict: false             # true일 때 작업 내 알 수 없는 필드는 경고 대신 오류 발생
  agent_type: opsmini       # agent.type에 기록됨, 기존 템플릿을 재사용하려면 filebeat로 설정
```

다중 호스트 시나리오에서 이러한 작업 파일은 OpsAnt가 gRPC를 통해 배포하며, 운영자는 플랫폼에서 구성하고 호스트 측에서는 수동으로 편집할 필요가 없습니다.

## 다른 모듈과의 관계

| 모듈 | 관계 |
|---|---|
| [자율 Agent(이벤트 센터)]](autonomous-agent.md) | 로그 폭증 수집기와 로그 파이프라인은 서로 다른 두 가지입니다. 전자는 '비정상적인 증가가 있는지'만 보며, 후자는 로그를 로그 플랫폼으로 전송하는 역할을 합니다 |
| [AI 어시스턴트]](ai-assistant.md) | AI의 `search_logs` 도구는 파이프라인에 의존하지 않고 로컬 로그 파일을 직접 읽습니다 |
| [설정 가이드 · 로그와 데이터]](../configuration/storage.md) | 데이터 디렉터리와 보존 정책 |
