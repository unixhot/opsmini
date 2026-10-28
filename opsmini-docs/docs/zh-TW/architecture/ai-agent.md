# AI 智慧體設計

> 里程碑 M8～M10 ｜ 程式碼：`internal/aiagent/`
>
> 套用方式：AI 助手 → 設定模型 → 直接用自然語言下指令

在 M8 中，OpsMini 的 AI 助手從「能讀取即時系統快照的維運顧問」升級為**真正能操作主機的智慧體**：模型自主規劃、呼叫工具、觀察結果並持續迭代直到得出結論——同時每一個高風險動作仍需人工明確核准。

## 1. 為什麼自研輕核心，而不是引入 Agent 框架

Go 生態中已有成熟的 Agent 框架（Google ADK Go、CloudWeGo Eino）。OpsMini 仍然自研約 1,500 行的核心，原因如下：

1. **護城河在領域能力，不在編排。** 框架解決的是通用的「模型＋工具」迴圈；OpsMini 的價值在於它的 29 個領域工具、分級安全護欄與 gRPC 多機通道，這些框架都不提供。
2. **膠水層的工作量與核心相當。** 工具都是普通的本機 Go 函式，把它們適配到框架的工具介面，工作量與自研差不多。
3. **單一二進位、無 CGO。** 引入 ADK 會把 OpenTelemetry／genai／GCS 一起拖進相依樹。
4. **用協定而非框架做擴充點。** 工具以 **MCP 相容的 JSON Schema** 宣告，絕不使用框架私有格式。附帶的好處是：同一份註冊表可以直接對外發布為 MCP Server。

因此選擇：**協定化工具 ＋ 自研核心**。若將來確實需要多智慧體協作或視覺化工作流編排，仍可切換到 Eino ADK 或 Google ADK Go，並透過 MCP／A2A 與 OpsMini 互通。

## 2. 核心結構

```
internal/aiagent/
├── types.go      Tool / Result / Message / ToolCall / RiskLevel
├── registry.go   執行緒安全註冊表（OpenAI 與 MCP 兩種定義）
├── llm.go        OpenAI 相容用戶端（串流 tool-call 碎片累積）
├── loop.go       ReAct 核心：推理 → 行動 → 觀察，支援中斷／恢復
├── guard.go      L3 硬禁止規則、面板自我保護、風險→策略對應
├── hitl.go       持久化、帶時效的人工核准
├── memory.go     對話持久化與上下文壓縮
├── mcpclient.go  MCP 用戶端（官方 Go SDK，stdio + Streamable HTTP）
├── mcpserver.go  MCP 服務端（把工具集發布給外部用戶端）
├── events.go     SSE 事件模型
└── tools/        內建工具集
```

## 3. ReAct 迴圈

一次使用者提問對應一次 `Agent.Send`：

1. 從 SQLite 載入對話記錄，前接系統提示詞與即時主機快照。
2. 以**串流**方式呼叫模型；文字增量作為 `thinking` 事件推送給前端。
3. 當模型要求呼叫工具時：
   - 先執行 **L3 硬禁止掃描**——命中即拒絕並把原因回灌給模型；
   - 再依風險等級與管理員設定的單工具策略決定：直接執行／需要確認／已停用；
   - 把觀察結果作為 `tool` 訊息回灌，繼續下一輪。
4. 當模型不再要求呼叫工具時，輸出最終回答。

每輪步數上限預設為 12；超過後智慧體會停止呼叫工具並回報已知資訊，避免任務失控。

## 4. 風險分級與安全護欄

| 等級 | 名稱 | 行為 |
|------|------|------|
| L0 | `RiskReadonly` | 唯讀，免確認自主執行 |
| L1 | `RiskLowWrite` | 低風險寫入；**預設仍需確認**，管理員可將個別工具放行為免確認 |
| L2 | `RiskHighWrite` | 高風險；**每次執行都需人工核准**，且永遠不可設為免確認 |
| L3 | 規則比對 | 在執行前即攔截，即使已核准也不執行 |

L3 覆蓋 `rm -rf /`（含關鍵系統目錄）、`mkfs`、`dd of=/dev/sd*`、重導向覆寫區塊裝置、關機／重啟、`chmod -R 777 /`、`chown -R … /`、fork 炸彈、清空 iptables／nft 規則、覆寫 `/etc/passwd` 一類的認證檔案、清除 shell 歷史、`rmmod` 與 `kill -9 1`。

另有兩條規則保護面板自身：

- 任何會刪除或覆寫面板資料目錄（二進位檔、SQLite 資料庫、技能）的操作一律拒絕；
- 會切斷面板監聽埠的防火牆規則一律拒絕。

> 產品判斷：不做無人值守的 root 智慧體。目標形態是**「自主規劃 ＋ 分級授權」**——AI 負責調查與建議，破壞性操作由人授權。

## 5. 人工確認（Human-in-the-loop）

當某次呼叫需要核准時，核心會寫入一筆 `ai_pending_approvals` 記錄（工作階段、工具、參數、風險等級、確認文案、到期時間——預設 10 分鐘），推送一個 `confirm` SSE 事件並**結束本輪**；前端據此顯示確認卡片。

使用者核准或拒絕後，`POST /ai/agent/approve` 會從資料庫還原完整訊息序列並繼續執行：

- 工具結果一律依呼叫順序持久化，且在遇到第一個未確認的呼叫時停止，因此**待確認呼叫的下標等於該則 assistant 訊息之後緊跟的 tool 訊息筆數**——恢復時不需要任何額外簿記。
- 同一輪中排在它之後的呼叫會被寫入明確的「未執行」結果，讓訊息序列對模型仍然合法。

## 6. 工具清單（29 個）

| 類別 | 數量 | 範例 |
|------|------|------|
| L0 唯讀 | 16 | `sys_overview`、`list_processes`、`list_ports`、`dir_usage`、`read_file`、`search_logs`、`query_metrics`、`docker_ps`、`security_status` |
| L1 低風險寫入 | 5 | `clean_temp`、`clean_logs`、`service_control`、`docker_control`、`install_skill` |
| L2 高風險寫入 | 8 | `exec_command`、`write_file`、`delete_path`、`modify_sshd`、`compress_dir`、`cron_write`、`kill_process`、`firewall_rule` |
| 外部 MCP | 動態 | `mcp__<server>__<tool>`，風險等級取自 `readOnlyHint` 標註 |

工具沿用面板的 RBAC：唯讀角色看不到需要寫入權限的工具，L2 工具另外需要 `ai.agent.write` 權限。

## 7. 雙向 MCP

### 作為 MCP 用戶端（使用外部工具）

在「AI 助手 → MCP」中設定 MCP 服務（stdio 命令，或帶自訂請求標頭的 Streamable HTTP 網址）。核心使用官方 MCP Go SDK 連線、列出工具，並把每個工具註冊為 `mcp__<server>__<tool>`。連線每兩分鐘對帳一次，因此改設定不需要重啟。

### 作為 MCP 服務端（對外提供能力）

面板在 `/mcp`（Streamable HTTP）發布自身的工具集：

- 認證使用**權杖（bearer token）**，首次啟動時產生，可在「AI 助手 → 工具權限」中檢視／複製／重設。MCP 用戶端無法完成面板的 JWT 登入，因此另設專用權杖。
- 預設只發布 **L0／L1 工具**。L2 工具在開啟「允許 MCP 高風險操作」之前不會下發，因為外部 MCP 用戶端無法呈現面板的確認對話框。
- L3 護欄與面板自我保護對遠端呼叫同樣生效。
- 每一次遠端呼叫都會寫入稽核日誌，操作者記為 `mcp-client`。

用戶端設定範例（Claude Desktop／Cursor 等 Streamable HTTP 用戶端）：

```json
{
  "mcpServers": {
    "opsmini": {
      "type": "http",
      "url": "https://your-panel.example.com/mcp",
      "headers": { "Authorization": "Bearer <token>" }
    }
  }
}
```

## 8. 工作階段與記憶

- `ai_sessions` / `ai_messages` 落 SQLite，完整記錄 `tool_calls` 與 `tool_call_id`，因此重新整理頁面或重啟行程都能恢復或回放思考鏈。
- 當歷史超過門檻（保留最近 24 則原文，外加 12 則緩衝）時會**壓縮**：更早的內容交由模型總結為不超過 300 字的摘要，作為一則 system 訊息插回。
- 工作階段預設保留 30 天（`ai_session_retention_days`），由背景定期清理。

## 9. 介面一覽

| 方法 | 路徑 | 用途 |
|------|------|------|
| POST | `/api/v1/ai/agent/chat` | 串流對話（思考鏈／工具呼叫／確認事件） |
| POST | `/api/v1/ai/agent/approve` | 核准或拒絕待確認操作並續跑 |
| GET | `/api/v1/ai/agent/tools` | 工具清單（風險、權限、目前策略）與 MCP 狀態 |
| PUT | `/api/v1/ai/agent/tools/policy` | 設定單一工具策略（免確認／需確認／停用） |
| GET | `/api/v1/ai/agent/sessions` | 工作階段清單 |
| GET | `/api/v1/ai/agent/sessions/:id/messages` | 對話訊息（含待確認記錄） |
| DELETE | `/api/v1/ai/agent/sessions/:id` | 刪除工作階段 |
| GET | `/api/v1/ai/agent/config` | 執行參數與模型可用性 |
| GET | `/api/v1/ai/agent/sessions/:id/report` | 匯出工作階段報告（預設 HTML） |
| GET | `/api/v1/ai/agent/reports` | 列出已歸檔報告（含格式與歸檔目錄） |
| GET | `/api/v1/ai/agent/reports/view?name=` | **直接開啟**已歸檔報告（inline，亦接受 `?token=`） |
| GET | `/api/v1/ai/agent/reports/download?name=` | 下載已歸檔報告（attachment，亦接受 `?token=`） |
| GET/PUT | `/api/v1/ai/agent/mcp-server` | 對外 MCP 端點設定（開關／權杖／高風險） |
| ANY | `/mcp` | 對外 MCP 服務端點（權杖認證） |

權限點：`ai.agent`（使用智慧體）與 `ai.agent.write`（授權 AI 執行高風險操作）。舊介面 `/ai/chat`、`/ai/chat/stream` 保留作為降級通道。

## 10. 多機能力歸屬：面板只做本機

OpsMini 是**單機面板**，AI 工具集只操作本機。多機操作屬於 OpsAnt —— 它才是中心，手上有 salt / ansible / opsmini 三個執行引擎與統一的執行 API（`/api/opsant/v1/tasks/run-shell/`、`tasks/run-script/`、`tasks/send-file/`、`tasks/get-minion-file/` 等）。

早期版本曾為面板加上 4 個多機工具（`remote_list_hosts`、`remote_exec`、`remote_pull_file`、`remote_push_file`），透過 OpsMini Server 的控制面 HTTP 介面（4508）反向派送任務。這些工具已移除，原因是它們跨越了架構邊界：

- 4508 是 OpsMini Server 給 OpsAnt（Django）使用的**內部**介面，定位為「僅本機 / 內網」，且不校驗權杖；
- 面板在這條鏈路上的角色是**被納管的 Agent**，只應透過 4507 的 gRPC 長連線與 Server 通訊（註冊、心跳、接收任務），不應反向呼叫控制面；
- 若要讓那個錯誤設計成立，就得把 4508 對外開放 —— 等於把一個無認證的遠端命令執行入口掛上公網。

因此分工是：**單機維運與單機 AI 在 OpsMini；多機編排與多機 AI 在 OpsAnt**。兩者以「OpsMini 作為 OpsAnt 的一種納管方式」銜接，資料流向始終是 OpsAnt → Agent。

## 11. 定時 AI 任務（主機巡檢等）

「AI 任務」不是新入口，而是**排程任務的第四種類型**（`kind = agent`），與「面板任務／系統 crontab／使用者 crontab」並列。因此排程、啟停、上次執行時間、失敗告警全部沿用現成機制，「每天早上自動做點什麼」就長在使用者原本就會去的地方。

### 執行鏈路

```
cron 觸發 -> 智慧體無人值守執行（ReAct，只讀／低風險工具）
          -> 對話落庫（報告即一個 AI 工作階段，含完整思考鏈）
          -> HTML 與 Markdown 歸檔到 <資料目錄>/reports/<任務名>/
          -> 摘要推送至任務設定的通知管道
```

### 無人值守時確認策略降級

定時執行時沒有人在旁邊點「核准」，因此當 `RunOptions.Unattended = true` 時：

- 命中 `PolicyConfirm`（L2）的工具**直接拒絕**；模型會被告知原因（「請改用唯讀工具取得資訊，並在結論中說明該操作需人工介入」），拒絕記錄寫入工作階段的 `Skipped` 清單；
- 模型據此改用唯讀工具，任務仍會跑完，報告中會寫明需要人工執行的操作與可直接複製的指令；
- L0 唯讀與 L1 低風險工具不受影響。因此「每日巡檢」在設計上就是唯讀的；要放寬必須明確調整策略。

拒絕項會寫入 `cron_jobs.skipped_tools`，並同時出現在通知與報告附錄中。

### 可靠性與成本控制

| 機制 | 說明 |
|------|------|
| 步數上限 | 任務級 `max_steps`（預設 12，可在表單調整） |
| 總逾時 | 任務級 `timeout_seconds`（預設 600 秒）；逾時按失敗處理 |
| 連續失敗自動停用 | 連續失敗 3 次自動停用任務，避免「每天失敗每天告警」 |
| 手動儲存即重置 | 在面板上儲存任務會把 `fail_count` 歸零 |

### 報告：單一檔案 HTML

每次執行產出**一個自包含的 HTML 檔**——這就是可直接開啟或下載的交付形態：

```
<資料目錄>/reports/<任務名>/<任務名>-<時間戳>.html   <- 報告本體（自包含）
<資料目錄>/reports/<任務名>/<任務名>-<時間戳>.md     <- 同一份內容的 Markdown
```

`<資料目錄>` 即面板設定中的資料目錄（`appstore_data_dir`，預設 `/var/lib/opsmini/data`）。**所有生成物都放在它底下、按功能分子目錄**——報告在 `reports/`，將來的生成型功能照此新增同層目錄即可。

「自包含」的含義是：樣式內嵌、無指令碼、無外部樣式表、無網路字型、無圖片。因此報告拷到隨身碟、當作郵件附件、放到任何靜態伺服器都能正常顯示。面板刻意**不內建中日韓字型**（否則單一二進位會增加 10 MB 以上），報告改用閱讀者系統自帶的字型——桌機與行動瀏覽器都有。

報告結構：執行摘要（狀態／步數／工具呼叫）→ 任務指令 → 結論 → **附：執行過程**（每次工具呼叫一行，參數與觀察結果並列，狀態為「完成／被拒絕／硬禁止／失敗」之一）。

任務清單每一列提供兩個入口：

- **開啟**——`GET /ai/agent/reports/view`，`Content-Disposition: inline`，在新分頁直接呈現；
- **下載**——`GET /ai/agent/reports/download`，`Content-Disposition: attachment`。

報告內文是模型產生的 Markdown，並混有主機資料（日誌片段、行程命令列、設定值）。渲染器採**先整體轉義、再套用行內規則**的順序，且不連結 `javascript:` 一類的位址，因此日誌裡出現 `<script>` 只會顯示為文字。回應另附嚴格 CSP 作為第二道防線。

需要 PDF 時，在報告頁面用瀏覽器「列印 → 另存為 PDF」即可——樣式表帶有列印規則，輸出效果與螢幕一致。

### 臨時對話也能匯出

AI 助手裡的任何對話都能匯出為同一套 HTML 報告：`GET /ai/agent/sessions/:id/report`（預設 HTML，`?format=md` 取 Markdown，`?download=1` 強制下載），因此未歸檔到磁碟的工作階段同樣能拿到可讀的報告。

### 通知沿用同一條鏈路

AI 任務與告警共用同一個 `Notifier`（`internal/service/notify.go`），涵蓋面板站內通知、郵件，以及企業微信／釘釘／飛書群機器人。管道清單為空時預設只發站內通知。
