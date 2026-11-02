# ログパイプライン

OpsMini には **Filebeat 設定と互換性のある**ログ収集実行端が組み込まれています。ホスト上のログをタスク設定に従って収集し、処理し、Elasticsearch / Kafka / Logstash へ配信します。位置づけとしては「管理対象ホスト上の filebeat」に相当しますが、別途プロセスをインストールする必要はありません。

## デフォルトでは無効

この点は最初に明確にしておく必要があります：**デフォルトのインストールではログを一切収集しません**。goroutine もなく、ディレクトリをスキャンせず、メモリも消費しません。

3段階の有効化があり、いずれか1つが成立すれば起動します：

| 方式 | トリガー条件 | 適用 |
|---|---|---|
| タスクファイル | `tasks.d` ディレクトリ内に任意の `.yml` / `.yaml` が存在する | 手動で単一ホストの収集を設定 |
| 明示的なスイッチ | `config.yaml` 内の `logpipeline.enabled: true` | 強制的に有効化したい場合（当面タスクがなくても） |
| プラットフォームからの配信 | OpsAnt から配信された `LogTask` を受信しディスクに保存 | 複数ホストの集中管理 |

逆に言えば、`enabled: false` かつタスクファイルがない場合、パイプラインは完全に停止します——これが「単一ホストの自己利用」シナリオにおけるデフォルト状態です。

## ディレクトリレイアウト

```
<データディレクトリ>/logs/
├── main.yml          # メイン設定（filebeat 形式：output + グローバルパラメータ）
├── tasks.d/          # 収集タスク、1タスク1ファイル
│   ├── nginx.yml
│   └── app-1024.yml
├── registry.db       # 各ファイルの読み取り位置（inode + offset）
└── spool/            # 出力失敗時のディスクキュー
```

`main.yml` は「どこへ送るか」を担当し、`tasks.d/*.yml` は「何を収集するか」を担当します。この分割は filebeat の `filebeat.yml` + `inputs.d/` と一致しているため、既存の filebeat 設定を分割してそのまま使用できます。

## タスクの書き方

**書き方 A（推奨）**：タスクは収集のみを記述し、出力は `main.yml` によって統一的に決定されます。

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

**書き方 B**：タスクファイルが `output` を独自に持ち、「タスクごとに異なる場所へ送信する」ために使用します。これは OpsMini が filebeat に対して拡張した点です——filebeat では複数のインスタンスが必要になります。

```yaml
output.elasticsearch:
  hosts: ["http://10.0.0.5:9200"]
  index: "app-%{+yyyy.MM.dd}"
```

どちらの書き方も受け付けられますが、B の優先度は `main.yml` より高くなります。

## ホットリロード

`tasks.d` は 10 秒ごとに照合します：

- **新規**タスクファイル → 解析後に収集を開始；解析失敗時は稼働中の旧バージョンを**上書きしない**ため、「1つのフィールドの修正ミスで収集が全停止する」ことを避けられます
- **変更**タスクファイル → 内容ハッシュが変わった場合のみ再構築し、同じ内容を繰り返し配信しても再起動は発生しません（再起動すると multiline バッファが失われ、すべてのファイルが開き直されます）
- **削除**タスクファイル → 該当タスクを停止します

タスクファイル名は `*.yml` / `*.yaml` のみ使用可能で、パスを含めることはできず、ドットで始めることもできません——ファイル名はリモートからディレクトリ外への書き出しに利用され得る唯一のフィールドであるため、直接制限しています。

## 対応プロセッサ

31 個の Filebeat プロセッサを搭載し、一般的なクレンジング要件をカバーします：

| カテゴリ | プロセッサ |
|---|---|
| フィールドの追加・削除・変更 | `add_fields` `add_tags` `drop_fields` `include_fields` `rename` `copy_fields` `move_fields` `truncate_fields` |
| 型と解析 | `convert` `decode_json_fields` `decode_csv_fields` `decode_duration` `dissect` `syslog` `decode_xml` `decode_base64_field` `decompress_gzip_field` `urldecode_fields` `extract_array` |
| 条件とフロー制御 | `drop_event` `script` |
| 時刻と識別子 | `timestamp` `add_id` `fingerprint` |
| メタデータ | `add_host_metadata` `add_observer_metadata` `add_locale` `add_network_direction` |
| ネットワークとドメイン | `community_id` `registered_domain` |
| テキスト置換 | `replace` |

## 出力

| 種類 | 説明 |
|---|---|
| `elasticsearch` | index テンプレート変数、アカウントパスワード、バッチサイズとタイムアウトをサポート |
| `kafka` | SASL 認証、topic 変数、パーティション戦略をサポート |
| `logstash` | Lumberjack v2 プロトコル、TLS 付き（`logstash` / `lumberjack` の両方の記法を認識） |

出力が未設定の場合、タスクは通常どおり収集を続けますが配信に失敗します。パネルはエラーをパイプラインステータスに表示します——サイレントに破棄されることはありません。

## データを失わない方法

配信セマンティクスは **at-least-once** です：

1. ログを一括で読み取る → 配信
2. **配信成功後**にのみ `registry.db` 内の offset を進める
3. 配信失敗 → `spool/` に書き込む（デフォルト上限 512MB）→ バックオフしてリトライ
4. プロセスがクラッシュ → 再起動後、最後に確認された offset から再送信する。重複する可能性はあるが、失われることはない

取引上、ログの一括重複は通常無害です（ES 側では `fingerprint` プロセッサで冪等化できます）。一方、ログの消失は取り返しがつかないため、前者を選びました。

## リアルタイムプレビュー

パネルの **ログ → パイプライン** タブで確認できます：

- パイプラインが稼働しているか、現在の出力タイプ、出力エラー
- 各タスクの収集ファイル数、読み取りレート、最終イベント時刻
- spool の滞留量
- 設定警告（未知のフィールド、存在しないパスなど）

タスクを選択すると**直近 20 件のイベントをリアルタイムプレビュー**でき、プロセッサチェーン通過後の形を確認できます——まず ES で調べる必要も、設定を変更して再起動する必要もありません。これは processor をデバッグする最速の方法です。

## なぜ Filebeat と互換性を持たせるのか

理由は三つあります：

1. **移行コスト**：既存の ES インデックステンプレート、ILM ポリシー、Kibana ダッシュボードをそのまま使い、`agent.type` は `filebeat` に設定して直接再利用できます
2. **運用上の可読性**：収集設定は運用担当者が最も慣れている YAML そのもので、新しい DSL を学ぶ必要がありません
3. **新たな依存関係を持ち込まない**：機能を単一バイナリに組み込み、ログ収集のために filebeat を追加インストールする必要がありません

## やらない部分

誤解を避けるため、正直に線を引いておきます：

- **module / autodiscover は行わない**（Nginx モジュール、Kubernetes 自動検出）：単一ホストのパネルにはコンテナオーケストレーションのコンテキストがない
- **ILM とインデックステンプレート管理は行わない**：それは ES 側の仕事
- **ホスト間の集約は行わない**：複数ホストの収集は OpsAnt がタスクを配信して統合管理し、OpsMini は実行端のみを担当
- **Filebeat のすべての出力はサポートしない**（Logstash Beat、Redis、Kafka 以外のミドルウェア）：ES / Kafka / Logstash の 3 種類のみ

## 設定リファレンス

```yaml
# config.yaml
logpipeline:
  enabled: false            # ハードスイッチ；true の場合はタスクファイルがなくても起動する
  tasks_dir: ""             # デフォルト <データディレクトリ>/logs/tasks.d
  config_file: ""           # メイン設定、デフォルト <データディレクトリ>/logs/main.yml
  strict: false             # true の場合、タスク内の未知のフィールドは警告ではなくエラーになる
  agent_type: opsmini       # agent.type に書き込まれる；filebeat に設定すると既存のテンプレートを再利用できる
```

マルチホスト環境では、これらのタスクファイルは OpsAnt によって gRPC 経由で配信され、オペレーターがプラットフォーム上で設定するため、ホスト側で手動編集する必要はありません。

## 他のモジュールとの関係

| モジュール | 関係 |
|---|---|
| [自律 Agent（イベントセンター）]](autonomous-agent.md) | ログ急増コレクターとログパイプラインは別物である：前者は「異常な増加があるかどうか」だけを見て、後者はログをログプラットフォームへ送る役割を担う |
| [AI アシスタント]](ai-assistant.md) | AI の `search_logs` ツールは本機のログファイルを直接読み取り、パイプラインに依存しない |
| [設定ガイド · ログとデータ]](../configuration/storage.md) | データディレクトリと保持ポリシー |
