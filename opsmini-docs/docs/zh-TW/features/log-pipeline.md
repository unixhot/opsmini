# 日誌管道

OpsMini 內建一個**與 Filebeat 配置相容**的日誌採集執行端。它把主機上的日誌按任務配置採集、處理、投遞到 Elasticsearch / Kafka / Logstash，位置相當於「被納管主機上的 filebeat」，但不需要另裝一個程序。

## 預設是關閉的

這一點必須先說清楚：**預設安裝不會採集任何日誌**。沒有 goroutine、不掃目錄、不佔記憶體。

三級啟用，任意一個成立就啟動：

| 方式 | 觸發條件 | 適用 |
|---|---|---|
| 任務檔案 | `tasks.d` 目錄裡出現任意 `.yml` / `.yaml` | 手工配置單機採集 |
| 顯式開關 | `config.yaml` 裡 `logpipeline.enabled: true` | 想強制開啟（哪怕暫時沒任務） |
| 平臺下發 | 收到 OpsAnt 下發的 `LogTask` 並落盤 | 多機集中管理 |

反過來說，`enabled: false` 且沒有任務檔案時，管道完全靜止——這是「單機自用」場景的預設狀態。

## 目錄佈局

```
<資料目錄>/logs/
├── main.yml          # 主配置（filebeat 形狀：output + 全域性引數）
├── tasks.d/          # 採集任務，一個任務一個檔案
│   ├── nginx.yml
│   └── app-1024.yml
├── registry.db       # 每個檔案的讀取位置（inode + offset）
└── spool/            # 輸出失敗時的磁碟佇列
```

`main.yml` 負責「往哪送」，`tasks.d/*.yml` 負責「採什麼」。這個拆分與 filebeat 的 `filebeat.yml` + `inputs.d/` 一致，所以已有的 filebeat 配置可以拆開直接用。

## 任務怎麼寫

**寫法 A（推薦）**：任務只描述採集，輸出統一由 `main.yml` 決定。

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

**寫法 B**：任務檔案自帶 `output`，用於「不同任務送不同地方」。這是 OpsMini 相對 filebeat 的擴充套件——filebeat 要多個例項才能做到。

```yaml
output.elasticsearch:
  hosts: ["http://10.0.0.5:9200"]
  index: "app-%{+yyyy.MM.dd}"
```

兩種寫法都接受，B 的優先順序高於 `main.yml`。

## 熱載入

`tasks.d` 每 10 秒對賬一次：

- **新增**任務檔案 → 解析後啟動採集；解析失敗**不覆蓋**正在執行的舊版本，避免「改錯一個欄位導致採集全停」
- **修改**任務檔案 → 內容雜湊變了才重建，重複下發同一份內容不會觸發重啟（重啟會丟 multiline 緩衝並重開所有檔案）
- **刪除**任務檔案 → 停止該任務

任務檔名只能是 `*.yml` / `*.yaml`，不能帶路徑、不能以點開頭——檔名是唯一可能被遠端用來寫出目錄的欄位，所以直接限死。

## 支援的處理器

31 個 Filebeat 處理器，覆蓋常見清洗需求：

| 類別 | 處理器 |
|---|---|
| 欄位增刪改 | `add_fields` `add_tags` `drop_fields` `include_fields` `rename` `copy_fields` `move_fields` `truncate_fields` |
| 型別與解析 | `convert` `decode_json_fields` `decode_csv_fields` `decode_duration` `dissect` `syslog` `decode_xml` `decode_base64_field` `decompress_gzip_field` `urldecode_fields` `extract_array` |
| 條件與流控 | `drop_event` `script` |
| 時間與標識 | `timestamp` `add_id` `fingerprint` |
| 後設資料 | `add_host_metadata` `add_observer_metadata` `add_locale` `add_network_direction` |
| 網路與域名 | `community_id` `registered_domain` |
| 文字替換 | `replace` |

## 輸出

| 型別 | 說明 |
|---|---|
| `elasticsearch` | 支援 index 模板變數、賬號密碼、批次大小與超時 |
| `kafka` | 支援 SASL 認證、topic 變數、分割槽策略 |
| `logstash` | Lumberjack v2 協議，帶 TLS（`logstash` / `lumberjack` 兩種寫法都認） |

未配置輸出時任務照常採集但投遞失敗，面板會把錯誤顯示在管道狀態裡——不會靜默丟棄。

## 不丟資料的做法

投遞語義是 **at-least-once**：

1. 讀到一批日誌 → 投遞
2. **投遞成功之後**才推進 `registry.db` 裡的 offset
3. 投遞失敗 → 寫進 `spool/`（預設上限 512MB）→ 退避重試
4. 程序崩潰 → 重啟後會從最後一次確認的 offset 重發，可能重複但不會丟

交易上重複一批日誌通常無害（ES 側可用 `fingerprint` 處理器做冪等），丟日誌則無法補救，所以選了前者。

## 實時預覽

面板 **日誌 → 管道** 標籤頁可以看到：

- 管道是否在跑、當前輸出型別、輸出錯誤
- 每個任務的採集檔案數、讀取速率、最後事件時間
- spool 積壓量
- 配置警告（未知欄位、路徑不存在等）

選中一個任務可以**實時預覽最近 20 條事件**，看處理器鏈之後長什麼樣——不用先去 ES 裡查，也不用改配置重啟。這是除錯 processor 最快的方式。

## 為什麼要相容 Filebeat

三個理由：

1. **遷移成本**：已有 ES 索引模板、ILM 策略、Kibana 看板不動，`agent.type` 還可以設成 `filebeat` 直接複用
2. **運維可讀**：採集配置是運維最熟悉的那套 YAML，不需要學新 DSL
3. **不引入新依賴**：能力做進單二進位制，不用為了採集日誌再裝 filebeat

## 不做的部分

誠實劃線，免得誤解：

- **不做 module / autodiscover**（Nginx 模組、Kubernetes 自動發現）：單機面板沒有容器編排上下文
- **不做 ILM 與索引模板管理**：那是 ES 側的事
- **不做跨主機匯聚**：多機採集由 OpsAnt 下發任務統一管理，OpsMini 只做執行端
- **不支援 Filebeat 的全部輸出**（Logstash Beat、Redis、Kafka 之外的中介軟體）：只做 ES / Kafka / Logstash 三種

## 配置參考

```yaml
# config.yaml
logpipeline:
  enabled: false            # 硬開關；true 時即使沒有任務檔案也啟動
  tasks_dir: ""             # 預設 <資料目錄>/logs/tasks.d
  config_file: ""           # 主配置，預設 <資料目錄>/logs/main.yml
  strict: false             # true 時任務裡的未知欄位直接報錯而不是警告
  agent_type: opsmini       # 寫進 agent.type；設成 filebeat 以複用已有模板
```

多機場景下這些任務檔案由 OpsAnt 透過 gRPC 下發，操作員在平臺上配置，主機側不需要手工編輯。

## 與其他模組的關係

| 模組 | 關係 |
|---|---|
| [自主 Agent（事件中心）](autonomous-agent.md) | 日誌暴漲採集器與日誌管道是兩件不同的事：前者只看「有沒有異常增長」，後者負責把日誌送到日誌平臺 |
| [AI 助手](ai-assistant.md) | AI 的 `search_logs` 工具直接讀本機日誌檔案，不依賴管道 |
| [配置指南 · 日誌與資料](../configuration/storage.md) | 資料目錄與保留策略 |
