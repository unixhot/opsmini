# 日志管道

OpsMini 内置一个**与 Filebeat 配置兼容**的日志采集执行端。它把主机上的日志按任务配置采集、处理、投递到 Elasticsearch / Kafka / Logstash，位置相当于「被纳管主机上的 filebeat」，但不需要另装一个进程。

## 默认是关闭的

这一点必须先说清楚：**默认安装不会采集任何日志**。没有 goroutine、不扫目录、不占内存。

三级启用，任意一个成立就启动：

| 方式 | 触发条件 | 适用 |
|---|---|---|
| 任务文件 | `tasks.d` 目录里出现任意 `.yml` / `.yaml` | 手工配置单机采集 |
| 显式开关 | `config.yaml` 里 `logpipeline.enabled: true` | 想强制开启（哪怕暂时没任务） |
| 平台下发 | 收到 OpsAnt 下发的 `LogTask` 并落盘 | 多机集中管理 |

反过来说，`enabled: false` 且没有任务文件时，管道完全静止——这是「单机自用」场景的默认状态。

## 目录布局

```
<数据目录>/logs/
├── main.yml          # 主配置（filebeat 形状：output + 全局参数）
├── tasks.d/          # 采集任务，一个任务一个文件
│   ├── nginx.yml
│   └── app-1024.yml
├── registry.db       # 每个文件的读取位置（inode + offset）
└── spool/            # 输出失败时的磁盘队列
```

`main.yml` 负责「往哪送」，`tasks.d/*.yml` 负责「采什么」。这个拆分与 filebeat 的 `filebeat.yml` + `inputs.d/` 一致，所以已有的 filebeat 配置可以拆开直接用。

## 任务怎么写

**写法 A（推荐）**：任务只描述采集，输出统一由 `main.yml` 决定。

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

**写法 B**：任务文件自带 `output`，用于「不同任务送不同地方」。这是 OpsMini 相对 filebeat 的扩展——filebeat 要多个实例才能做到。

```yaml
output.elasticsearch:
  hosts: ["http://10.0.0.5:9200"]
  index: "app-%{+yyyy.MM.dd}"
```

两种写法都接受，B 的优先级高于 `main.yml`。

## 热加载

`tasks.d` 每 10 秒对账一次：

- **新增**任务文件 → 解析后启动采集；解析失败**不覆盖**正在运行的旧版本，避免「改错一个字段导致采集全停」
- **修改**任务文件 → 内容哈希变了才重建，重复下发同一份内容不会触发重启（重启会丢 multiline 缓冲并重开所有文件）
- **删除**任务文件 → 停止该任务

任务文件名只能是 `*.yml` / `*.yaml`，不能带路径、不能以点开头——文件名是唯一可能被远程用来写出目录的字段，所以直接限死。

## 支持的处理器

31 个 Filebeat 处理器，覆盖常见清洗需求：

| 类别 | 处理器 |
|---|---|
| 字段增删改 | `add_fields` `add_tags` `drop_fields` `include_fields` `rename` `copy_fields` `move_fields` `truncate_fields` |
| 类型与解析 | `convert` `decode_json_fields` `decode_csv_fields` `decode_duration` `dissect` `syslog` `decode_xml` `decode_base64_field` `decompress_gzip_field` `urldecode_fields` `extract_array` |
| 条件与流控 | `drop_event` `script` |
| 时间与标识 | `timestamp` `add_id` `fingerprint` |
| 元数据 | `add_host_metadata` `add_observer_metadata` `add_locale` `add_network_direction` |
| 网络与域名 | `community_id` `registered_domain` |
| 文本替换 | `replace` |

## 输出

| 类型 | 说明 |
|---|---|
| `elasticsearch` | 支持 index 模板变量、账号密码、批量大小与超时 |
| `kafka` | 支持 SASL 认证、topic 变量、分区策略 |
| `logstash` | Lumberjack v2 协议，带 TLS（`logstash` / `lumberjack` 两种写法都认） |

未配置输出时任务照常采集但投递失败，面板会把错误显示在管道状态里——不会静默丢弃。

## 不丢数据的做法

投递语义是 **at-least-once**：

1. 读到一批日志 → 投递
2. **投递成功之后**才推进 `registry.db` 里的 offset
3. 投递失败 → 写进 `spool/`（默认上限 512MB）→ 退避重试
4. 进程崩溃 → 重启后会从最后一次确认的 offset 重发，可能重复但不会丢

交易上重复一批日志通常无害（ES 侧可用 `fingerprint` 处理器做幂等），丢日志则无法补救，所以选了前者。

## 实时预览

面板 **日志 → 管道** 标签页可以看到：

- 管道是否在跑、当前输出类型、输出错误
- 每个任务的采集文件数、读取速率、最后事件时间
- spool 积压量
- 配置警告（未知字段、路径不存在等）

选中一个任务可以**实时预览最近 20 条事件**，看处理器链之后长什么样——不用先去 ES 里查，也不用改配置重启。这是调试 processor 最快的方式。

## 为什么要兼容 Filebeat

三个理由：

1. **迁移成本**：已有 ES 索引模板、ILM 策略、Kibana 看板不动，`agent.type` 还可以设成 `filebeat` 直接复用
2. **运维可读**：采集配置是运维最熟悉的那套 YAML，不需要学新 DSL
3. **不引入新依赖**：能力做进单二进制，不用为了采集日志再装 filebeat

## 不做的部分

诚实划线，免得误解：

- **不做 module / autodiscover**（Nginx 模块、Kubernetes 自动发现）：单机面板没有容器编排上下文
- **不做 ILM 与索引模板管理**：那是 ES 侧的事
- **不做跨主机汇聚**：多机采集由 OpsAnt 下发任务统一管理，OpsMini 只做执行端
- **不支持 Filebeat 的全部输出**（Logstash Beat、Redis、Kafka 之外的中间件）：只做 ES / Kafka / Logstash 三种

## 配置参考

```yaml
# config.yaml
logpipeline:
  enabled: false            # 硬开关；true 时即使没有任务文件也启动
  tasks_dir: ""             # 默认 <数据目录>/logs/tasks.d
  config_file: ""           # 主配置，默认 <数据目录>/logs/main.yml
  strict: false             # true 时任务里的未知字段直接报错而不是警告
  agent_type: opsmini       # 写进 agent.type；设成 filebeat 以复用已有模板
```

多机场景下这些任务文件由 OpsAnt 通过 gRPC 下发，操作员在平台上配置，主机侧不需要手工编辑。

## 与其他模块的关系

| 模块 | 关系 |
|---|---|
| [自主 Agent（事件中心）](autonomous-agent.md) | 日志暴涨采集器与日志管道是两件不同的事：前者只看「有没有异常增长」，后者负责把日志送到日志平台 |
| [AI 助手](ai-assistant.md) | AI 的 `search_logs` 工具直接读本机日志文件，不依赖管道 |
| [配置指南 · 日志与数据](../configuration/storage.md) | 数据目录与保留策略 |
