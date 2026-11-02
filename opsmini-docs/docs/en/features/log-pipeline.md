# Log Pipeline

OpsMini ships a log collection executor **compatible with Filebeat configuration**. It collects, processes and ships logs to Elasticsearch / Kafka / Logstash according to task files — the role a filebeat instance would play on a managed host, without installing another process.

## Off by default

This must be said first: **a default install collects no logs at all**. No goroutine, no directory scan, no memory.

Three ways to enable it, any one is enough:

| Method | Trigger | Use for |
|---|---|---|
| Task file | any `.yml` / `.yaml` appears in `tasks.d` | manual single-host collection |
| Explicit switch | `logpipeline.enabled: true` in `config.yaml` | force it on even with no tasks yet |
| Platform dispatch | a `LogTask` arrives from OpsAnt and is written to disk | central multi-host management |

Conversely, with `enabled: false` and no task files the pipeline is completely inert — which is exactly the right default for a standalone install.

## Directory layout

```
<data dir>/logs/
├── main.yml          # main config (filebeat-shaped: output + globals)
├── tasks.d/          # collection tasks, one file per task
│   ├── nginx.yml
│   └── app-1024.yml
├── registry.db       # read position per file (inode + offset)
└── spool/            # disk queue for output failures
```

`main.yml` decides *where to ship*, `tasks.d/*.yml` decides *what to collect*. This split mirrors filebeat's `filebeat.yml` + `inputs.d/`, so existing filebeat configs can be split and reused.

## Writing a task

**Option A (recommended)**: the task describes collection only; output is decided globally.

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

**Option B**: the task carries its own `output`, for "different tasks to different places". This is an OpsMini extension — filebeat needs multiple instances to do the same.

```yaml
output.elasticsearch:
  hosts: ["http://10.0.0.5:9200"]
  index: "app-%{+yyyy.MM.dd}"
```

Both are accepted; B takes precedence over `main.yml`.

## Hot reload

`tasks.d` is reconciled every 10 seconds:

- **Added** task file → parsed and started; a parse failure **does not overwrite** the running version, so one bad field cannot take collection down
- **Changed** task file → rebuilt only when the content hash changes; re-sending identical content does not restart (a restart loses multiline buffers and reopens every file)
- **Removed** task file → that task stops

Filenames must end in `.yml` / `.yaml`, must not contain a path, must not start with a dot — the filename is the only field a remote caller could use to write outside the directory, so it is locked down.

## Processors

31 Filebeat processors covering the usual cleaning needs:

| Category | Processors |
|---|---|
| Field edits | `add_fields` `add_tags` `drop_fields` `include_fields` `rename` `copy_fields` `move_fields` `truncate_fields` |
| Types & parsing | `convert` `decode_json_fields` `decode_csv_fields` `decode_duration` `dissect` `syslog` `decode_xml` `decode_base64_field` `decompress_gzip_field` `urldecode_fields` `extract_array` |
| Conditional & flow | `drop_event` `script` |
| Time & identity | `timestamp` `add_id` `fingerprint` |
| Metadata | `add_host_metadata` `add_observer_metadata` `add_locale` `add_network_direction` |
| Network & domains | `community_id` `registered_domain` |
| Text | `replace` |

## Outputs

| Type | Notes |
|---|---|
| `elasticsearch` | index template variables, basic auth, batch size and timeout |
| `kafka` | SASL, topic variables, partition strategy |
| `logstash` | Lumberjack v2 with TLS (`logstash` and `lumberjack` both accepted) |

With no output configured, tasks still collect but delivery fails and the error is shown in the pipeline status — nothing is silently dropped.

## How data is not lost

Delivery is **at-least-once**:

1. Read a batch of lines → ship it
2. Advance the offset in `registry.db` **only after the output accepted it**
3. On failure → write to `spool/` (512MB default) → retry with backoff
4. On crash → resume from the last confirmed offset and re-send; duplicates are possible, loss is not

Duplicated log batches are usually harmless (use the `fingerprint` processor for idempotency on the Elasticsearch side); lost logs cannot be recovered, so the trade-off goes that way.

## Live preview

**Logs → Pipeline** tab shows:

- whether the pipeline is running, current output type, output errors
- per task: files collected, read rate, last event time
- spool backlog
- configuration warnings (unknown fields, missing paths)

Select a task to **preview the last 20 events in real time** and see what they look like after the processor chain — without querying Elasticsearch and without editing config. This is the fastest way to debug a processor.

## Why Filebeat compatibility

Three reasons:

1. **Migration cost** — existing index templates, ILM policies and Kibana dashboards stay as they are; `agent.type` can even be set to `filebeat` to reuse them
2. **Operational familiarity** — collection config is the YAML operators already know
3. **No extra dependency** — the capability lives in the single binary; no filebeat install required

## What is not implemented

An honest line, to avoid misunderstanding:

- **No modules / autodiscover** (Nginx module, Kubernetes discovery): a single-host panel has no orchestration context
- **No ILM or index template management**: that belongs to Elasticsearch
- **No cross-host aggregation**: multi-host collection is dispatched by OpsAnt; OpsMini is the executor
- **Not every Filebeat output** (Logstash Beat, Redis, other brokers): only ES / Kafka / Logstash

## Configuration reference

```yaml
# config.yaml
logpipeline:
  enabled: false            # hard switch; true starts it even with no task files
  tasks_dir: ""             # default <data dir>/logs/tasks.d
  config_file: ""           # main config, default <data dir>/logs/main.yml
  strict: false             # true makes unknown task fields an error instead of a warning
  agent_type: opsmini       # written to agent.type; set filebeat to reuse templates
```

In multi-host setups these task files are dispatched by OpsAnt over gRPC — operators configure them on the platform and never hand-edit the host.

## Related

| Module | Relationship |
|---|---|
| [Autonomous agent](autonomous-agent.md) | The log-growth collector and the log pipeline are different things: one watches for abnormal growth, the other ships logs to a platform |
| [AI assistant](ai-assistant.md) | The `search_logs` tool reads local files directly and does not depend on the pipeline |
| [Configuration · Data storage](../configuration/storage.md) | Data directory and retention |
