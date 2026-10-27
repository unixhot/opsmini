# OpsMini 自主 Agent 架构手册（v1.3.0）

> 面向二次开发者与运维负责人。设计意图见 [v1.3.0 设计文档](v1.3.0-autonomous-agent.md)；本文只讲**实现细节**：数据怎么流、每个采集器怎么判断、护栏在哪一层生效、出问题怎么排查、怎么加新场景。

---

## 1. 全景：五个平面与三条路径

```
采集平面  internal/sentinel/*.go
  logwatch  authwatch  filewatch  procwatch  netwatch  metricwatch
        │  Signal（统一信号）
        ▼
归一平面  sentinel.Bus → sentinel.Aggregator
  去重(dedup_key, 30min) → 落库(signals) → 聚合(incidents, 10min 窗口)
        │  Incident + Signal
        ▼
决策平面  internal/trigger
  Match(family+severity) → cooldown → 并发闸(1) → 组装 RunOptions → 起会话
        │  trigger.RunRequest
        ▼
行动平面  internal/aiagent + internal/aiagent/tools/respond.go
  取证(只读) → 判定 → 处置(限定工具集) → 验证(断言) → 报告
        │  ActionEvent（每次工具决策）
        ▼
兜底平面  internal/safety
  ArgPolicy / Quarantine / Snapshot / Budget / KillSwitch / Verify
```

三条触发路径共用同一个 Agent 内核：

| 路径 | 入口 | RunOptions 关键字段 |
|---|---|---|
| 事件触发 | `trigger.Engine.OnIncident` → `incidentRunner.RunIncident` | `Mode`、`AllowedTools`、`MaxWrites`、`Evidence`、`IncidentID` |
| 定时巡检 | `service/cron.go`（`kind=agent`）→ `scheduledRunner.RunUnattended` | `Unattended=true` |
| 人工对话 | `POST /api/v1/ai/agent/chat` | 无额外限制（受 RBAC 与 L0–L3 约束） |

---

## 2. 数据流与生命周期

### 2.1 Signal → Incident → AgentRun

```
采集器产生 Signal
   ├─ Bus.handle：dedupe_key 命中窗口内 → 直接丢弃（不落库）
   ├─ 否则落 signals 表
   └─ 广播给 Aggregator
          ├─ 找同 family+subject 且 10 分钟内活跃的 incident
          │    ├─ 找到：count++ / last_seen / severity 取 max
          │    └─ 没有：新建 incident（状态 open）
          ├─ 把 signal.incident_id 回填
          └─ 广播给 trigger.Engine
                 ├─ 策略未命中 / 冷却中 / 并发已占用 → 只留 incident（open）
                 └─ 命中：incident → triggered → running → resolved/failed
                       同时写 agent_runs 一行
```

**状态机**

| 状态 | 含义 | 谁写入 |
|---|---|---|
| `open` | 有信号、未触发 | Aggregator |
| `triggered` | 策略命中，等并发闸 | Engine |
| `running` | Agent 正在跑 | Engine |
| `resolved` | 跑完且验证通过（或无断言） | Engine |
| `failed` | 跑失败 / 验证未通过 | Engine |
| `ignored` | 人工忽略（同时写 signal_feedback） | API |
| `taken_over` | 人工接管，Agent 不得再动 | API |

### 2.2 为什么事件要单独存一张表

`signals` 是**证据**，`incidents` 是**问题**。分开的原因有三：

1. 去重与聚合发生在不同粒度（信号 30 分钟、事件 10 分钟窗口）。
2. 事件详情页需要按时间回放原始信号（含 evidence JSON），不能只有聚合结果。
3. 信号 7 天滚动淘汰，事件保留 90 天——生命周期不同。

---

## 3. 采集器实现细节

所有采集器都实现同一个接口，跑在独立 goroutine 里，输出走 `func(Signal)`：

```go
type collector interface {
    Name() string
    Run(ctx context.Context, out func(Signal))
}
```

**共同约定**
- 单次扫描不得超过 200 个文件 / 20000 个 fd，超出即 `filepath.SkipAll`，防止在异常主机上自伤。
- 所有阈值来自 `sentinel.Config`，由 panel settings 覆盖，改配置**不需要重启**（下一轮扫描生效）。
- 采集器自身不做判断之外的任何写操作。

### 3.1 logwatch —— 日志暴涨

| 项 | 实现 |
|---|---|
| 采样 | 每 10s 对 `/var/log` 下文件名含 `log` 的常规文件取 `stat` |
| 排除 | `lastlog/wtmp/btmp/utmp/faillog`、`.gz/.xz/.1/.2`、`journal/private/old` 目录 |
| 速率 | `delta = size - prev.size`；`perMin = delta * 60 / 10` |
| 基线 | 按「小时桶」维护 EWMA（α=0.3，含方差），每桶独立 |
| 硬阈值 | `perMin >= 20MB`：**不受学习期限制**，连续 2 个采样点即触发 |
| 软阈值 | `samples >= 20` 且 `(cur - mean)/max(sigma,4KB) >= 3`，连续 2 点 |
| 升级 | 硬阈值命中，或偏离 ≥6σ，或尾部含 error/fatal/panic → high |
| 反查持有者 | 扫 `/proc/*/fd` 找正在写该文件的 pid + cmdline（决定「能不能动」） |
| 模板指纹 | 末 8KB 逐行归一化（数字→N、IP/uuid/hex→占位），单模板占比 >80% 且行数 ≥10 → 判定"刷日志"并升级 |
| 轮转识别 | inode 变化或 size 变小 → 重置基线，不产生信号 |

**为什么硬阈值不受学习期限制**：学习期的目的是压住"正常业务波动"，而 20MB/min 的增量本身就不是正常波动。若把它也压住，第一天上线的机器会漏掉最明显的故障。

### 3.2 authwatch —— 登录行为

| 项 | 实现 |
|---|---|
| 数据源 | `/var/log/auth.log` 或 `/var/log/secure`（谁存在用谁），增量读，单次上限 1MB |
| 解析 | 三条正则：`Accepted <method> for <user> from <ip>`、`Failed ... for <user> from <ip>`、`Invalid user <u> from <ip>` |
| 特征 | off_hours（可配 0–6 点）、new_ip（baselines 表 `auth_ip:<ip>` 从未出现）、recent_failures（该 IP 10 分钟内失败数）、authorized_keys 30 分钟内是否被改（filewatch 也独立报） |
| 判定 | 失败后成功（≥5 次）→ **critical**；key 刚改就登录 → **critical**；非工作时段+新 IP → **high**；仅新 IP / 仅非工作时段 → **medium**；其余 → info（仍落库，便于回溯） |
| 失败爆发 | 单 IP 1 小时内失败 ≥20 次 → medium（`auth.failed_burst`），计数后清零，避免刷屏 |
| 备注 | 只支持文件日志；若发行版只写 journald，authwatch 静默退出（不影响其它采集器） |

### 3.3 filewatch —— 落地文件与关键配置

| 项 | 实现 |
|---|---|
| Web 根 | 默认 `/var/www,/var/www/html,/www/wwwroot,/data/www`（可配），每 20s 走一遍，上限 2 万文件 |
| 只关心 | 扩展名 `.php .phtml .php5 .jsp .jspx .aspx .ashx .asp .cgi .pl .py .sh` |
| 变化判定 | (mtime,size) 缓存；首次扫描只建索引不报警 |
| 内容评分 | 读头部 64KB，命中 14 类特征；**强特征**（`eval($_POST`、`gzinflate(base64`、`preg_replace("/e`、`/dev/tcp/`）任一命中 → high；否则命中 ≥2 → high，命中 1 → medium |
| 额外 | `/root/.ssh/authorized_keys` mtime 变化 → `auth.key_changed`（high），与 authwatch 交叉验证 |

### 3.4 procwatch —— 进程链

| 项 | 实现 |
|---|---|
| 采样 | 每 2s 遍历 `/proc`（读 comm/cmdline/status/stat） |
| 规则 1 | comm 命中挖矿名单（`minerd/kdevtmpfsi/xmrig/kinsing/...`）→ **critical** |
| 规则 2 | cmdline 命中反弹特征（`/dev/tcp/`、`bash -i`、`nc -e`、`socat`、`exec 5<>`）→ **critical** |
| 规则 3 | **web 用户（www-data/nginx/apache/httpd/tomcat）派生出 shell/curl/wget/nc/python/perl/ruby/php** → **critical**，带父进程 cmdline |
| 去重 | `fingerprint:pid` 30 分钟内只报一次 |

**已知局限**：2s 轮询会漏掉更短命的进程。要覆盖毫秒级 exec，需要 auditd 或 eBPF，v1.3 明确不做（见设计文档"明确不做"）。实践中 webshell 派生 shell 往往不是毫秒级，因此该漏报可接受。

### 3.5 netwatch —— 出站连接

| 项 | 实现 |
|---|---|
| 采样 | 每 20s 读 `/proc/net/tcp{,6}` 取 `ESTABLISHED`（state 01） |
| 归属 | 扫 `/proc/*/fd` 的 `socket:[inode]` 映射到 pid（上限 2 万 fd） |
| 规则 1 | 远端端口命中 `4444/1337/31337/6667/5555/9999/12345` → **high** |
| 规则 2 | web 用户连非常规端口（非 80/443/3306/6379/...）→ medium |

### 3.6 metricwatch —— 复用现有告警引擎

不重复采集。`service.AlertMonitor` 触发告警时调用 `SetOnFire` 注册的钩子，钩子把告警转成 `Signal{kind=metric, family=metric}` 塞进同一条通道。

这样做的副作用是：**现有告警规则自动成为 Agent 的感知源**，无需再配一遍。

---

## 4. 触发层

### 4.1 策略模型（`trigger_policies`）

```yaml
name: webshell-respond        # 唯一名，冷却按它计
family: webshell              # 匹配 incident.family
min_severity: high            # 达到即触发
mode: assist                  # shadow | assist | auto
cooldown_sec: 3600            # 同一策略的两次触发间隔
max_writes: 4                 # 单次运行写动作上限
max_steps: 12                 # ReAct 步数上限
max_seconds: 300              # 墙钟上限（context timeout）
max_tokens: 150000            # 预留：token 预算
allow_tools: [...]            # 工具白名单（空=全部）
deny_tools: [exec_command]    # 硬拒绝，优先级高于白名单
auto_tools: [quarantine_file] # 仅 auto 模式生效：可免确认的 L1 工具
verify: [{type: no_new_signal, family: webshell, window: 30m}]
notify: panel,email
prompt: |                     # playbook 正文（{title} 占位）
```

内置 5 条策略（首次启动 seed）：`webshell-respond`（assist）、`auth-anomaly`（assist）、`proc-anomaly`（assist，critical 起）、`net-anomaly`（shadow，默认关闭）、`log-growth`（shadow）。

### 4.2 触发闸门

```
Match(family, severity)
  → Enabled?
  → 冷却（engine.last[policy]，内存态，进程重启后重置）
  → 并发闸 sem(1)：单机不允许两个 agent 同时改机器；占用中直接放弃本次
  → 急停检查：paused 时 mode 强制降为 shadow
  → 写 agent_runs(status=running)
  → go run（带 timeout）
```

**为什么并发是 1**：单机上两个 Agent 同时处置会互相破坏对方的取证与验证。宁可排队（下一次信号会重触发），也不并发。

### 4.3 RunOptions 的组装

`BuildRequest` 阶段把策略翻译成 Agent 能理解的约束，关键有三条：

1. `AllowedTools` 非空时，未列出的工具**根本不会出现在模型的工具列表里**（不是"调了再拒"）。
2. `Evidence` 把已确认的事实拼成一条 system message，省掉 3–5 轮盲查。
3. `OnAction` 回调把每一次工具决策写成 `action_log`，这是决策链审计的来源。

---

## 5. 行动层：工具与三种模式

### 5.1 工具分层

| 层 | 名称 | 例子 | 谁来拦 |
|---|---|---|---|
| L0 | 只读 | `sys_overview`、`read_file`、`search_logs` | 放行 |
| L1 | 可逆写（**新增**） | `quarantine_file`、`freeze_process`、`block_ip`、`snapshot_path`、`truncate_log` | `PolicyFor` → 默认 confirm；auto 模式下白名单内直接执行 |
| L2 | 高危写 | `exec_command`、`write_file`、`delete_path`、`kill_process`、`modify_sshd` | 必须人工确认（无人在场即拒绝） |
| L3 | 禁止 | 命中 `guard.forbiddenArgs` 的命令、动面板自身端口 | 执行前硬拒绝 |

**事件触发模式下 `exec_command` 默认不在白名单**：webshell 场景的 `deny_tools` 里明确写着它。这比任何提示词约束都可靠。

### 5.2 三种模式的真实行为

| 模式 | L0 | L1 | L2 | 写动作效果 |
|---|---|---|---|---|
| shadow | 执行 | **不执行**，只记 `verdict=shadow` | 不执行 | 无（可在动作审计看到"打算做什么"） |
| assist | 执行 | 拒绝并记入 skipped（需人工） | 拒绝 | 无 |
| auto | 执行 | `auto_tools` 内的执行，其余拒绝 | 拒绝 | 有 |

> v1.3.0 的 assist 是"只读处置建议"：Agent 把要做的事写清楚，人去点。真正的"点一下继续执行"（HITL 续跑并回填事件状态）排在 v1.3.1。

### 5.3 六个可逆原语（`internal/safety`）

| 原语 | 语义 | 逆操作 |
|---|---|---|
| `Quarantine(path)` | 复制到隔离区 → `chmod 000` → 删除原文件，记录 sha256/权限/属主 | `RestoreQuarantine(id)`：目标已存在则拒绝覆盖 |
| `Snapshot(path)` | 复制到快照区，记录 sha256 | `RestoreSnapshot(id)` |
| `freeze_process` | `SIGSTOP` | `resume_process`：`SIGCONT` |
| `block_ip(ttl)` | `iptables -I INPUT -s ip -j DROP`（无 iptables 则 `nft`） | TTL 到期自动解封 + `unblock_ip` |
| `truncate_log(keep)` | 先快照，再保留末 N 行写回（文件 inode 不变，持有进程不报 EBADF） | 用快照回滚 |
| `service_action` | 复用现有 `service_control`，受保护服务集拦截 | 记录原状态，失败自动回退（v1.3.1） |

### 5.4 ArgPolicy：参数级约束

工具名相同、参数不同，危险程度天差地别。`safety.CheckArg` 在每个写工具入口被调用：

```
protected_paths  /etc/ssh /etc/passwd /etc/shadow /etc/sudoers /boot /lib/modules /data/opsmini ...
protected_units  sshd systemd-journald systemd-logind networking opsmini cron ...
protected_procs  systemd init sshd opsmini（+ pid 1 / 自身 pid）
deny_cmd_patterns rm -rf / · mkfs · dd of=/dev/ · shutdown/reboot · :(){ · chmod -R 777 /
```

命中即 `deny`，并写入 `action_log.rule`（例如 `protected-path:/etc/ssh`），事后能查出是哪条规则拦的。

### 5.5 验证（Assertion DSL）

```jsonc
{"type":"no_new_signal","family":"webshell","window":"30m"}
{"type":"service_active","unit":"nginx"}
{"type":"port_listen","port":80}
{"type":"process_absent","pattern":"minerd"}
{"type":"disk_usage_lt","path":"/var","value":85}
```

执行时机：Agent 会话结束、且没报错时，由 Engine 调用 `safety.Verify`。
- 全过 → incident `resolved`
- 任一失败 → incident `failed`，`verify_state=failed`，通知里带"⚠ 验证未通过，需人工确认"
- 连续两次失败 → 该 incident 熔断，不再重试

---

## 6. 行动平面：Agent 内核的改动

`internal/aiagent/loop.go` 只加了四件事，没有重写内核：

| 改动 | 位置 | 说明 |
|---|---|---|
| 工具过滤 | `filterTools(available, allow, deny)` | 白名单/黑名单在**提供给模型之前**生效 |
| 证据注入 | `evidenceBlock(opt)` | 采集器事实作为 system message |
| 影子与预算 | 循环内 `tool.Risk >= RiskLowWrite` 分支 | 影子模式全部拒绝；写次数超预算拒绝 |
| 动作回调 | `runTool` 尾部 + 拒绝分支 | 每次决策与结果写 `action_log` |

`PolicyFor` 的默认语义不变（L1 仍需确认），auto 模式通过 `override="auto"` 白名单式放开，且**只对 `RiskLowWrite` 生效**——L2 永远不受 `auto_tools` 影响。

---

## 7. 数据模型

| 表 | 主键 | 关键字段 | 保留 |
|---|---|---|---|
| `signals` | id(32) | kind, source, subject, severity, fingerprint, family, dedup_key, evidence(JSON), incident_id | 7 天 |
| `incidents` | id(32) | family, subject, severity, status, count, policy_name, mode, session_id, report_rel, summary, verify_state | 90 天 |
| `trigger_policies` | id | name, family, min_severity, mode, cooldown_sec, max_writes/steps/seconds, allow/deny/auto_tools, verify, prompt | 永久 |
| `agent_runs` | id | incident_id, mode, status, session_id, steps/tools/writes, skipped, report_rel, error | 90 天 |
| `action_log` | id | incident_id, session_id, tool, args, risk, verdict, rule, result, reversible, duration_ms | 180 天 |
| `quarantine` | id | incident_id, original_path, stored_path, sha256, size, mode, owner, restored_at | 30 天 |
| `snapshots` | id | path, stored_path, kind, sha256, rolled_back | 30 天 |
| `baselines` | key | kind, value, sigma, extra（如 `auth_ip:<ip>`） | 永久 |
| `signal_feedback` | id | fingerprint, verdict(false_positive/true_positive), by | 永久 |
| `blocked_ips` | id | ip, reason, expires_at, released_at | 自动清理 |

**磁盘占用**：证据只存摘要 + 大字段落盘（预留 `evidence/` 目录）；隔离区与快照目录在 `<data>/safety/{quarantine,snapshots}/<incident>/`，按 30 天清理。

---

## 8. 接口与前端

| 方法 | 路径 | 权限 | 用途 |
|---|---|---|---|
| GET | `/sentinel/stats` | ai.agent | 首页四数字 + 采集器状态 + 急停位 |
| GET | `/sentinel/signals` | ai.agent | 信号流（kind/severity/family 过滤） |
| GET | `/sentinel/incidents` | ai.agent | 事件列表 |
| GET | `/sentinel/incidents/:id` | ai.agent | 详情（signals + actions + runs） |
| POST | `/sentinel/incidents/:id/ignore` | ai.agent | 忽略（写 signal_feedback） |
| POST | `/sentinel/incidents/:id/takeover` | ai.agent | 接管 |
| GET/PUT | `/sentinel/policies[/:id]` | ai.agent / settings.edit | 策略读写（模式切换） |
| POST | `/sentinel/pause` | settings.edit | 急停 / 恢复 |
| GET | `/sentinel/actions` | ai.agent | 动作审计 |
| GET | `/sentinel/quarantine` · POST `/:id/restore` · DELETE `/:id` | ai.agent | 隔离区 |
| GET | `/sentinel/blocks` · DELETE `/:host/ip` | ai.agent | 封禁列表 / 解封 |
| GET | `/sentinel/facets` | ai.agent | 筛选词表：分类计数、等级、场景清单、处置工具、模式 |
| POST | `/sentinel/policies` · DELETE `/sentinel/policies/:id` | settings.edit | 新建 / 删除策略 |
| GET/PUT | `/sentinel/retention` | ai.agent / settings.edit | 数据保留天数与各表行数 |
| POST | `/sentinel/retention/purge` | settings.edit | 立即清理（`dry_run` 可只预览） |

所有列表接口统一分页：`?page=&page_size=`（默认 20，上限 200），返回
`{list, total, page, page_size}`。事件额外支持 `status`（含虚拟值 `unresolved` = open+triggered+running+failed）、
`family`、`severity`、`q`；动作支持 `tool`、`verdict`、`q`；信号支持 `kind`、`severity`、`family`、`q`。

前端：侧边栏「事件中心」页，五个 Tab（事件 / 策略 / 动作审计 / 隔离与封禁 / 信号流），顶部常驻急停开关。
文案在 `I18N9` + `I18N10`（后者置于 dicts 首位，用于覆盖旧文案），7 语言各 118 键对齐。

---

## 9. 配置项（面板设置）

| key | 默认 | 说明 |
|---|---|---|
| `sentinel_enabled` | 空（=启用） | `false` 关闭整个感知层 |
| `sentinel_learning` | 空（=关闭） | `true` 学习期：只记录信号、不触发 Agent |
| `sentinel_log_paths` | `/var/log` | 逗号分隔 |
| `sentinel_web_roots` | `/var/www,/var/www/html,/www/wwwroot,/data/www` | 逗号分隔 |

改完即生效（下一轮扫描读取）。策略与模式在 UI 里改，落 `trigger_policies`。

---

## 10. 排查手册

| 现象 | 先看什么 | 常见原因 |
|---|---|---|
| 事件中心一直是空的 | `GET /sentinel/stats` 的 `collectors` 与 `signals_24h` | ① `sentinel_enabled=false`；② 日志路径下没有符合条件的文件；③ 学习期开着只有 open 事件 |
| 有信号没事件 | `GET /sentinel/signals` 的 `incident_id` 是否为空 | Aggregator 落库失败（看服务日志 `sentinel: create incident`） |
| 有事件没 Agent 运行 | `trigger_policies` 的 `enabled/mode/min_severity`；`agent_runs` 是否为空 | ① 等级没到阈值；② 冷却中（`last_run_at`）；③ 并发被占（日志 `trigger: busy`）；④ 模型未配置 |
| Agent 跑了但什么都没做 | `GET /sentinel/actions` 的 verdict 分布 | shadow：全是 `shadow`；assist：全是 `deny`（需人工） |
| 处置被拒绝 | `action_log.rule` | `protected-path:*` / `protected-unit:*` / `kill-switch` / `write-budget` |
| 误报太多 | `signal_feedback` + 具体 fingerprint | 调策略阈值，或把该场景 mode 降回 shadow；把路径加进配置排除 |
| 验证总失败 | `agent_runs.summary` 里的"验证：" | 断言与实际情况不符（例如写死了 port 80 但服务在 8080） |
| 封禁没生效 | `iptables -S INPUT | grep DROP` | 无 iptables 权限 / 使用了 nft 后端；`blocked_ips.expires_at` 是否已过期 |

**日志关键字**：`sentinel:`（采集/总线）、`trigger:`（策略/调度）。

---

## 11. 扩展指南

### 加一个采集器

1. 在 `internal/sentinel/` 新建 `xxxwatch.go`，实现 `Name()` 与 `Run(ctx, out func(Signal))`。
2. 用 `newSignal(...)` 构造，务必给稳定的 `Fingerprint` 与 `Family`（family 决定策略匹配）。
3. 大证据用 `withEvidence(map[string]any{...})`，单条上限 8KB（超出会被丢弃，避免撑爆上下文）。
4. 在 `manager.go` 的 `m.collectors` 里注册。

### 加一个处置动作

1. 优先复用 safety 原语；若必须新增系统调用，先回答：**逆操作是什么**。答不出来就不要做成 L1。
2. 在 `tools/respond.go` 注册工具：`Risk: aiagent.RiskLowWrite`，入口调 `d.Safety.CheckArg`。
3. 如需自动执行，把工具名加进策略的 `auto_tools`，并确认它幂等、可回滚。
4. 给该场景写一条 `verify` 断言——没有验证的动作等于没做完。

### 加一个场景（策略）

在 `trigger/defaultPolicies()` 里加一条，或直接在 UI 里改现有策略。要点：`family` 必须与采集器产出的 `Family` 一致；`deny_tools` 里把 `exec_command` 列上（除非你确实要给它通用执行能力）。

---

## 12. 与 v1.2.1 的兼容性

- 只新增表，不修改既有表结构与语义；升级无需迁移。
- 现有 `kind=agent` 定时任务、AI 助手、MCP、报告、通知全部不受影响。
- 默认配置下上线后：事件中心开始有数据，Agent 默认 shadow/assist，**不会自动改动任何东西**。
- `tool_count` 从 29 变为 38（新增 9 个处置工具）。

---

## 13. 已知限制（诚实清单）

1. 无 eBPF/内核探针：毫秒级进程、内存马、容器逃逸看不到。
2. 只解析文件形式的 auth 日志；纯 journald 的发行版需要自行配 `rsyslog` 落盘。
3. 基线在内存中重建：进程重启后 logwatch 需要重新学习（硬阈值不受影响）。
4. 单会话并发：极端情况下多场景同时发生会排队，后面的靠下一次信号重触发。
5. assist 模式不续跑：写动作只出建议，需人工在 AI 助手里执行（v1.3.1 补 HITL 续跑）。
6. `truncate_log` 会重写文件内容（保留尾部），对正在做 `tail -f` 的进程无影响，但会丢失前端输出——因此它只在 auto 白名单里按场景开放。

---

## 12. 面板：如何使用事件中心

> 本节面向使用者，可直接作为官方文档的「事件中心」章节。

### 12.1 四个数字都能点

| 卡片 | 含义 | 点击后 |
|---|---|---|
| 24h 信号 | 采集器最近一天产出的原始观测数 | 跳到「信号流」 |
| 待处置 | 还需要人看一眼的事件数（open + triggered + running + failed） | 跳到「事件」，按待处置筛选 |
| 24h 事件 | 最近一天聚合出的事件数 | 跳到「事件」，全部 |
| 24h 动作 | 最近一天 Agent 的工具调用数 | 跳到「动作审计」 |

「待处置」和「24h 事件」差在哪：已处置、已忽略、已接管的那部分不计入待处置。数字对不上时先看这里。

### 12.2 事件怎么分类

事件按**场景（family）**分类，只有六种，与采集器一一对应：

| family | 中文 | 来源采集器 | 典型触发 |
|---|---|---|---|
| `webshell` | WebShell | filewatch | web 目录新增脚本 + 内容特征，或进程链交叉命中 |
| `auth.anomaly` | 登录异常 | authwatch | 非工作时段、新 IP、爆破后成功、authorized_keys 被改 |
| `proc.anomaly` | 可疑进程 | procwatch | 挖矿特征、反弹 shell、web 用户派生 shell |
| `net.anomaly` | 异常外联 | netwatch | 可疑端口、已知 C2 端口、异常出站 |
| `log_growth` | 日志暴涨 | logwatch | 小时桶 EWMA 偏离 3σ，或超过 20MB/min |
| `metric` | 指标告警 | metricwatch | 复用既有告警引擎，命中阈值即入总线 |

分类下拉里的计数来自 `/sentinel/facets`，是**全量**计数，不受当前状态筛选影响。

### 12.3 事件详情里有什么

点行首的「详情」：

- **摘要**：Agent 的结论（assist/auto 模式跑过才有）
- **关联信号**：这条事件由哪些观测聚合而成，每条带等级与原文
- **运行记录**：几次 Agent 运行、各用了多少步/工具/写操作
- **动作链**：每一次工具调用 + 裁决 + 结果（`allow` / `deny` / `shadow` / `confirm`）

判定「Agent 到底动没动手」看动作链：`verdict=allow` 且 `risk=low_write/high_write` 才是真写了。

---

## 13. 面板：策略怎么配

### 13.1 一句话

策略是**数据**，回答两件事：这类事件要不要叫醒 Agent、叫醒后它最多能做什么。表 `trigger_policies`，UI 可增删改，改完下一轮判定即生效，不用重启。

### 13.2 唤醒的三个条件（AND）

```
事件分类 == 策略 family
且 严重级 >= 策略 min_severity
且 距上次同策略运行 >= cooldown_sec
```

不满足就只记录事件、不消耗模型额度。这是控制成本的主要开关。

### 13.3 三种模式

| 模式 | 只读工具 | 写动作 | 适合 |
|---|---|---|---|
| `shadow` | 自动 | **一律不执行**，只记「本来会做什么」 | 新场景上线前跑几天看判断准不准 |
| `assist` | 自动 | 一律等人工确认（事件触发的运行里直接拒绝并记入 skipped） | 默认值，日常最稳 |
| `auto` | 自动 | 只有 `auto_tools` 白名单里的 L1 动作直接执行 | 已跑稳的场景，且只放可回滚动作 |

`auto` 不会突破 L0–L3 分级：L2 以上永远需要人，`auto_tools` 里填 L2 工具也不会生效。

### 13.4 字段速查

| 字段 | 类型 | 说明 | 常见错误 |
|---|---|---|---|
| `name` | 唯一字符串 | 创建后不可改 | 重名会报「策略名已存在」 |
| `family` | 六选一 | 必须与采集器场景一致 | 自定义值永远匹配不上事件 |
| `min_severity` | info…critical | 越低越敏感也越贵 | 调到 `info` 会让每个正常登录都唤醒 Agent |
| `mode` | shadow/assist/auto | 见上表 | 新场景直接上 `auto` |
| `cooldown_sec` | 0–86400 | 同策略两次运行最小间隔 | 设 0 等于不限制，同类事件会连续唤醒 |
| `max_writes` | 0–20 | 一次运行最多写几次 | 设太小会让 Agent 做完取证就没法处置 |
| `max_steps` / `max_seconds` / `max_tokens` | — | 单次运行的步数/时长/token 上限 | 步数太小会「查一半就停」 |
| `allow_tools` | JSON 数组 | 下发给模型的工具全集 | 忘记放 `read_file` 会导致没法取证 |
| `auto_tools` | JSON 数组 | 仅 auto 模式生效的免确认工具 | 填了 L2 工具不生效（安全设计） |
| `verify` | JSON 数组 | 动作后的断言，见 §5.5 | 写成 `{}` 会被拒绝；空字符串表示不验证 |
| `prompt` | 文本 | 处置 playbook，`{title}` 会被替换 | 写太笼统，模型容易乱查 |

### 13.5 建议的上线节奏

1. 新场景先 `shadow` 跑 3–7 天，看事件详情里的「本来会做什么」是否符合预期；
2. 判断稳定后切 `assist`，此时只读自动、写要人点；
3. 同一场景连续 7 天没有误判，再把 1–2 个可回滚动作放进 `auto_tools` 并切 `auto`；
4. 任何时候都保留急停（右上角 Pause）：急停后所有运行强制降级为 shadow。

---

## 14. 数据保留与清理

### 14.1 为什么要有

信号是高频写入（这台测试机一天 50 条左右，繁忙主机可能上万），动作审计是每次工具调用一条。不清理的话 SQLite 会一直长。

### 14.2 规则（重要）

| 表 | 默认保留 | 清理条件 |
|---|---|---|
| `signals` | 14 天 | 超期 **且** 所属事件已关闭（或没挂事件） |
| `incidents` | 90 天 | 超期 **且** 状态属于 resolved/ignored/taken_over/failed |
| `actions` | 180 天 | 超期即删（审计留最久） |
| `agent_runs` | 90 天 | 超期且所属事件已关闭 |
| `quarantine` | 30 天 | **只删已还原**的记录行，文件本身不碰 |
| `blocked_ips` | 30 天 | **只删已解封**的记录行 |

三条硬规则：

1. **待处置事件永不自动删**（open/triggered/running），它和它的信号、动作链都是证据；
2. **隔离文件在还原前不动**，避免把可能要还原的东西弄丢；
3. **封禁在解封前不动**，避免记录没了但 iptables 规则还在。

设 `0` 表示永久保留。所有天数在「隔离与封禁 → 数据保留与清理」里改，落 `settings.sentinel_retention`，重启不丢。

### 14.3 什么时候跑

后台每小时一次（`startRetention`，首次延迟 3 分钟避开启动高峰），日志会打
`sentinel: retention removed signals=N incidents=N ...`。

手动触发：`POST /sentinel/retention/purge {"what":"all","dry_run":true}` 先看数量，
再把 `dry_run` 设 false。UI 上就是「预览可清理量」和「立即清理」两个按钮。

### 14.4 容量估算

单条例行的 SQLite 开销（含索引）大致：signal ≈ 0.8 KB、action ≈ 1.2 KB、incident ≈ 2 KB。
按每天 1 万条信号算，14 天约 110 MB，加上索引不到 200 MB —— 对单机面板是可接受量级。
