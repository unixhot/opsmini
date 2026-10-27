<div align="center">

# OpsMini AI 智能体设计（AI Agent）

**AI Agent Design**

[English](ai-agent.md) · [简体中文](ai-agent.zh-CN.md) · [繁體中文](ai-agent.zh-TW.md) · [日本語](ai-agent.ja.md) · [한국어](ai-agent.ko.md) · [ไทย](ai-agent.th.md) · [Deutsch](ai-agent.de.md)

</div>

---

> 版本：M8 ｜ 对应代码：`internal/aiagent/`

OpsMini 的 AI 助手在 M8 阶段从「能读到实时系统快照的运维顾问」升级为**具备真实操作能力的智能体**：模型可以自主规划、调用工具、观察结果并迭代，直到得出结论；所有高风险动作都必须经过人工确认。

## 1. 为什么自研轻内核，而不是引入 Agent 框架

Go 生态已有成熟的 Agent 框架（Google ADK Go、字节 CloudWeGo Eino 等），OpsMini 仍然选择自研约 1500 行内核，理由：

1. **护城河不在编排，而在领域能力。** 框架解决的是通用的「模型 + 工具循环」，而 OpsMini 的价值在 29 个领域工具、分级安全护栏、gRPC 多机通道——这些框架给不了。
2. **胶水成本约等于自研成本。** 工具全是本地 Go 函数，把它们适配成框架的 tool 接口，代码量与本项目的 ReAct 循环相当。
3. **单二进制 + 无 CGO 是交付卖点。** 引入 ADK 会带进 OpenTelemetry / genai / GCS 等依赖树。
4. **用协议做扩展点，而不是用框架。** 工具以 **MCP 兼容的 JSON Schema** 声明，不绑定任何框架私有格式；因此 OpsMini 既能作为 MCP 客户端接入外部工具，也能**把自身工具集反向发布为 MCP Server**（白送的生态能力）。

结论：**工具协议化 + 自研内核**。当未来需要多 Agent 协作、可视化工作流编排时，可切换到 Eino ADK 或 Google ADK Go，届时通过 MCP / A2A 与 OpsMini 互通即可。

## 2. 内核结构

```
internal/aiagent/
├── types.go      Tool / Result / Message / ToolCall / RiskLevel
├── registry.go   线程安全工具注册表（转 OpenAI 定义 / MCP 定义）
├── llm.go        OpenAI 兼容客户端（流式 tool_calls 碎片累积）
├── loop.go       ReAct 内核：reason → act → observe，支持中断与恢复
├── guard.go      L3 硬禁止规则 + 面板自保护 + 风险→策略映射
├── hitl.go       人工确认（持久化 + 过期）
├── memory.go     会话持久化与上下文压缩
├── mcpclient.go  MCP 客户端（官方 Go SDK，stdio + Streamable HTTP）
├── mcpserver.go  MCP 服务端（把工具集发布给外部客户端）
├── events.go     SSE 事件模型
└── tools/        内置工具集
```

## 3. ReAct 循环

一次用户提问对应一次 `Agent.Send`：

1. 载入会话历史（SQLite），拼上系统提示词与实时主机快照；
2. 调用模型（**流式**），把文本增量作为 `thinking` 事件推给前端；
3. 若模型发起工具调用：
   - 先过 **L3 硬禁止扫描**，命中即拒绝并回灌给模型；
   - 再按风险等级与管理员策略决定「直接执行 / 需人工确认 / 禁用」；
   - 执行后把结果作为 `tool` 消息回灌，继续下一轮；
4. 模型不再调用工具时输出最终答案。

单次提问的步数上限默认 12 步；超出后停止继续调用工具，并把已知信息作为结论返回，避免失控。

## 4. 风险分级与安全护栏

| 等级 | 名称 | 行为 |
|------|------|------|
| L0 | `RiskReadonly` | 只读，自主执行 |
| L1 | `RiskLowWrite` | 低危写，**默认仍需确认**，管理员可对单个工具开启免确认 |
| L2 | `RiskHighWrite` | 高危，**每次都必须人工批准**，且不可设为免确认 |
| L3 | 模式匹配 | 执行前硬禁止，即使管理员批准也不会执行 |

L3 覆盖的危险模式包括：`rm -rf /`（含系统关键目录）、`mkfs`、`dd of=/dev/sd*`、重定向覆盖块设备、关机/重启、`chmod -R 777 /`、`chown -R ... /`、fork 炸弹、清空 iptables/nft 规则、覆盖 `/etc/passwd` 等认证文件、清除命令历史、`rmmod`、`kill -9 1`。

**面板自保护**（额外两条）：

- 任何会删除或覆盖面板数据目录（二进制、SQLite 库、技能目录）的操作被拒绝；
- 任何会阻断面板监听端口的防火墙规则被拒绝。

> 产品判断：不做无人值守的 root 自主 Agent。目标形态是「**自主规划 + 分级授权**」——AI 负责查证与建议，破坏性动作由人拍板。

## 5. 人工确认（Human-in-the-loop）

需要确认时，内核把待确认记录写入 `ai_pending_approvals`（含会话、工具、参数、风险、预览文案与过期时间，默认 10 分钟），向 SSE 推送 `confirm` 事件后**结束本轮**；前端弹出确认卡片。

用户点击「批准执行 / 拒绝」后调用 `POST /ai/agent/approve`，内核从数据库恢复完整消息序列并续跑：

- 由于工具结果严格按调用顺序落库、并在第一个未确认调用处停止，**待确认调用的下标 = 该 assistant 消息之后紧跟的 tool 消息条数**——恢复时无需额外簿记即可精确定位。
- 同一轮里排在它后面的调用会补上「未执行」的结果，保证消息序列对模型始终合法。

## 6. 工具集（29 个）

| 分类 | 数量 | 代表工具 |
|------|------|---------|
| L0 只读 | 16 | `sys_overview`、`list_processes`、`list_ports`、`dir_usage`、`read_file`、`search_logs`、`query_metrics`、`docker_ps`、`security_status` |
| L1 低危写 | 5 | `clean_temp`、`clean_logs`、`service_control`、`docker_control`、`install_skill` |
| L2 高危写 | 8 | `exec_command`、`write_file`、`delete_path`、`modify_sshd`、`compress_dir`、`cron_write`、`kill_process`、`firewall_rule` |
| 外部 MCP | 动态 | `mcp__<server>__<tool>`，风险等级由 `readOnlyHint` 注解推导 |

工具继承面板的 RBAC：只读角色看不到需要写权限的工具，L2 工具额外要求 `ai.agent.write` 权限。

## 7. 双向 MCP

### 作为 MCP 客户端（接入外部工具）

在「AI 助手 → MCP」中配置 MCP 服务（stdio 命令或 Streamable HTTP 地址 + 自定义请求头），内核使用官方 MCP Go SDK 连接并列出工具，逐个注册为智能体工具，名字前缀 `mcp__<服务名>__<工具名>`。连接每 2 分钟对账一次，配置变更无需重启。

### 作为 MCP Server（对外提供能力）

面板把自身工具集发布在 `/mcp`（Streamable HTTP）：

- 使用 **Bearer Token** 鉴权（首次启动自动生成，可在「AI 助手 → 工具权限」查看、复制、重置）；MCP 客户端无法走面板 JWT 登录，因此使用独立令牌；
- 默认**只发布 L0 / L1 工具**；L2 工具在未开启「允许 MCP 高危操作」时不下发——外部客户端无法呈现面板的确认弹窗；
- L3 硬禁止与面板自保护对远程调用同样生效；
- 所有远程调用写入审计日志（操作者记为 `mcp-client`）。

配置示例（Claude Desktop / Cursor 等支持 Streamable HTTP 的客户端）：

```json
{
  "mcpServers": {
    "opsmini": {
      "type": "http",
      "url": "https://your-panel.example.com/mcp",
      "headers": { "Authorization": "Bearer <令牌>" }
    }
  }
}
```

## 8. 会话与记忆

- `ai_sessions` / `ai_messages` 落 SQLite，完整记录 `tool_calls` 与 `tool_call_id`，因此刷新页面、重启进程都能续跑或回放思考链；
- 历史超过阈值时（默认保留最近 24 条 + 12 条缓冲）自动**压缩**：把更早的消息交给模型生成 300 字以内的要点摘要，作为一条 system 消息插回；
- 会话默认保留 30 天（`ai_session_retention_days`），后台定期清理。

## 9. 接口一览

| 方法 | 路径 | 说明 |
|------|------|------|
| POST | `/api/v1/ai/agent/chat` | SSE 流式对话（思考链 / 工具调用 / 确认事件） |
| POST | `/api/v1/ai/agent/approve` | 批准或拒绝待确认操作并续跑（SSE） |
| GET | `/api/v1/ai/agent/tools` | 工具清单（风险、权限、当前策略）与 MCP 连接状态 |
| PUT | `/api/v1/ai/agent/tools/policy` | 按工具设置策略（免确认 / 需确认 / 禁用） |
| GET | `/api/v1/ai/agent/sessions` | 会话列表 |
| GET | `/api/v1/ai/agent/sessions/:id/messages` | 会话消息（含待确认记录） |
| DELETE | `/api/v1/ai/agent/sessions/:id` | 删除会话 |
| GET | `/api/v1/ai/agent/config` | 运行参数与模型可用性 |
| GET | `/api/v1/ai/agent/sessions/:id/report` | 下载该会话的 Markdown 报告 |
| GET | `/api/v1/ai/agent/reports` | 列出已归档的报告文件（含 format 与归档目录） |
| GET | `/api/v1/ai/agent/reports/view?name=` | **直接打开**归档报告（inline，也接受 `?token=`） |
| GET | `/api/v1/ai/agent/reports/download?name=` | 下载归档报告（attachment，也接受 `?token=`） |
| GET/PUT | `/api/v1/ai/agent/mcp-server` | 对外 MCP 端点配置（开关 / 令牌 / 高危开关） |
| ANY | `/mcp` | 对外 MCP 服务端点（Bearer Token 鉴权） |

权限点：`ai.agent`（使用智能体）、`ai.agent.write`（授权 AI 执行高风险操作）。旧接口 `/ai/chat`、`/ai/chat/stream` 保留作为降级通道。

## 10. 多机能力归属：面板只做本机

OpsMini 是**单机面板**，AI 工具集只操作本机。多机操作属于 OpsAnt —— 它才是中心，手里有 salt / ansible / opsmini 三个执行引擎和统一的执行 API（`/api/opsant/v1/tasks/run-shell/`、`tasks/run-script/`、`tasks/send-file/`、`tasks/get-minion-file/` 等）。

早期版本曾给面板加过 4 个多机工具（`remote_list_hosts`、`remote_exec`、`remote_pull_file`、`remote_push_file`），通过 OpsMini Server 的控制面 HTTP 接口（4508）反向下发任务。这些工具已移除，原因是它们越过了架构边界：

- 4508 是 OpsMini Server 给 OpsAnt（Django）用的**内部**接口，定位「仅本机 / 内网」，且不校验鉴权；
- 面板在这条链路上的角色是**被纳管的 Agent**，只应通过 4507 的 gRPC 长连接与 Server 通信（注册、心跳、接收任务），不应反向调用控制面；
- 为了让那个错误设计跑通，就得把 4508 暴露出来 —— 等于把一个无认证的远程命令执行入口挂到公网。

因此分工是：**单机运维与单机 AI 在 OpsMini；多机编排与多机 AI 在 OpsAnt**。两者通过「OpsMini 作为 OpsAnt 的一种纳管方式」衔接，数据流向始终是 OpsAnt → Agent。

## 11. 定时 AI 任务（主机巡检等）

「AI 任务」不是新入口，而是**计划任务的第四种类型**（`kind = agent`），与「面板任务 / 系统 crontab / 用户 crontab」并列。这样调度、启停、上次执行时间、失败告警全部复用现成机制，用户想「每天早上自动做点什么」时只需去计划任务页。

### 执行链路

```
cron 触发 → Agent 无人值守执行（ReAct + 只读/低危工具）
         → 会话落库（报告 = 一个 AI 会话，自带完整思考链）
         → 归档 Markdown 到 <data_dir>/reports/<任务名>/
         → 推送摘要到任务配置的通知渠道
```

### 无人值守的确认策略降级

定时任务没有人在旁边点「批准」，因此 `RunOptions.Unattended = true` 时：

- 命中 `PolicyConfirm`（L2 高危）的工具**直接拒绝**，把拒绝原因回灌给模型（"请改用只读工具获取信息，并在结论中说明该操作需要人工介入"），并记入会话的 `Skipped` 列表；
- 模型据此换用只读工具，任务继续跑完，报告里会明确写出「该操作需人工执行」并给出可复制的命令；
- L0 只读与 L1 低危工具不受影响。这样「每天巡检」天然是只读的；想放开某类写操作，需要管理员在工具权限页显式调整策略。

拒绝情况会写进 `cron_jobs.skipped_tools` 并出现在通知与报告附录中。

### 可靠性与成本控制

| 机制 | 说明 |
|------|------|
| 步数上限 | 任务级 `max_steps`（默认 12，可在表单里调） |
| 总超时 | 任务级 `timeout_seconds`（默认 600 秒），超时按失败处理 |
| 连续失败自动停用 | 连续失败 3 次自动禁用任务，避免「每天失败每天告警」 |
| 手动保存复位 | 在面板上保存任务会把 `fail_count` 清零 |

### 报告：单文件 HTML

每次执行产出**一个自包含的 HTML 文件**——这是要交付的形态，可以在浏览器里直接打开，也可以下载归档：

```
<数据目录>/reports/<任务名>/<任务名>-<时间戳>.html     ← 单文件报告（自包含）
<数据目录>/reports/<任务名>/<任务名>-<时间戳>.md       ← 同一份内容的 Markdown（便于脚本处理与再渲染）
```

`<数据目录>` 就是面板设置里的数据目录（`appstore_data_dir`，默认 `/var/lib/opsmini/data`），
**所有生成物都落在它下面，按功能分子目录**——报告在 `reports/`，将来其它生成型功能也照此新增子目录即可。

「自包含」的含义：样式内联、无脚本、无外部样式表、无网络字体、无图片。因此报告拷到 U 盘、作为
邮件附件、放到任意静态服务器上都能正常显示。**面板不内置中文字体**（否则单二进制要涨 10 MB 以上），
所以报告使用阅读者的系统字体——桌面和移动端浏览器都自带。

报告结构：执行摘要（状态 / 步数 / 工具调用）→ 任务指令 → 结论 → **附：执行过程**（每次工具调用一行，参数与观察结果并列，状态含"完成 / 被拒绝 / 硬禁止 / 失败"）。

任务列表每行提供两个入口：

- **打开**——`GET /ai/agent/reports/view`，`Content-Disposition: inline`，在新标签页直接渲染；
- **下载**——`GET /ai/agent/reports/download`，`Content-Disposition: attachment`。

正文是模型生成的 Markdown，其中混有主机数据（日志片段、进程命令行、配置值）。渲染器采用
**先整体转义、再套用行内规则**的顺序，并且不链接 `javascript:` 之类的地址，因此日志里出现
`<script>` 只会显示为文本。响应另附严格 CSP，作为第二道防线。

需要 PDF 时，在报告页面用浏览器「打印 → 另存为 PDF」即可——样式表里带了打印规则，输出效果与屏幕一致。

### 临时会话也能导出

AI 助手里的任意会话都可以导出为同一套 HTML 报告：
`GET /ai/agent/sessions/:id/report`（默认 HTML，`?format=md` 取 Markdown，`?download=1` 强制下载）。
未归档到磁盘的会话因此也能拿到一份可读的报告。

### 通知复用

AI 任务与告警共用同一个 `Notifier`（`internal/service/notify.go`），支持面板站内、邮件、企业微信 / 钉钉 / 飞书机器人。渠道为空时默认只发站内通知。
