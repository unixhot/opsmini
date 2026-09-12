# Prometheus 지표（/metrics）

OpsMini는 **node_exporter 호환** `/metrics` 엔드포인트를 내장하고 있어 node_exporter를 별도로 설치할 필요 없이 Prometheus가 직접 scrape할 수 있습니다.

## 활성화와 인증

`config.yaml`에서 활성화합니다（기본값 활성화）:

```yaml
metrics:
  enabled: true      # /metrics 엔드포인트 활성화 여부
  user: ""           # Basic 인증 사용자 이름, 비어 있으면 인증 없음
  password: ""       # Basic 인증 비밀번호
```

인증 자격 증명 우선순위: 패널「설정 → 모니터링 내보내기」의 사용자 이름/비밀번호가 **설정 파일보다 우선합니다**; 사용자 이름이 비어 있으면 인증 없음입니다.

## Prometheus 설정

```yaml
scrape_configs:
  - job_name: 'opsmini'
    static_configs:
      - targets: ['<host>:8888']
    metrics_path: '/metrics'
    basic_auth:            # 인증을 활성화한 경우 해당 자격 증명을 설정해야 합니다
      username: 'monitor'
      password: '<password>'
```

## 지표 패밀리

node_exporter의 핵심 지표에 정렬되어 있어 커뮤니티 Node Dashboard를 그대로 사용할 수 있습니다:

| 지표 패밀리 | 설명 |
|--------|------|
| `node_cpu_seconds_total{cpu,mode}` | 코어별 모드별 누적 초 |
| `node_memory_MemTotal_bytes` 등 | 메모리 Total / Free / Available / Buffers / Cached |
| `node_filesystem_size_bytes{mountpoint}` | 마운트 포인트별 용량 / 가용 / 사용률 |
| `node_network_receive_bytes_total{device}` | 네트워크 인터페이스별 송수신 트래픽 |
| `node_load1` / `node_load5` / `node_load15` | 부하 |
| `node_uname_info` / `node_boot_time_seconds` | 호스트 정보와 부팅 시간 |

## 설명

- 구현은 `gopsutil`이 이미 수집한 데이터를 재사용하여 Prometheus 텍스트 형식으로 출력합니다
- 단일 바이너리 제공을 유지하며 node_exporter 프로세스를 내장하지 않습니다
- `server.secret_entry`를 설정했다면 엔드포인트 역시 프리픽스 아래에 마운트됩니다（예: `/opsmini_panel/metrics`）
