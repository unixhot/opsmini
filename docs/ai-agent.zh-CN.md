# OpsMini AI 智能体设计（AI Agent）

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
| 多机（需配置） | 4 | `list_remote_hosts`、`remote_exec`、`remote_pull_file`、`remote_push_file` |
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
| GET/PUT | `/api/v1/ai/agent/mcp-server` | 对外 MCP 端点配置（开关 / 令牌 / 高危开关） |
| ANY | `/mcp` | 对外 MCP 服务端点（Bearer Token 鉴权） |

权限点：`ai.agent`（使用智能体）、`ai.agent.write`（授权 AI 执行高风险操作）。旧接口 `/ai/chat`、`/ai/chat/stream` 保留作为降级通道。

## 10. 多机统一管理

当面板配置了 `agent.server_addr`（指向 OpsAnt 上的 OpsMini Server 数据面）后，内核自动注册 4 个多机工具，通过 Server 的控制面 HTTP 接口（默认同主机的 4508 端口）操作其它被纳管主机：命令/脚本下发、文件拉取与下发，全程免 SSH。地址与令牌可用 `ai_remote_server_url` / `ai_remote_server_token` 覆盖。
