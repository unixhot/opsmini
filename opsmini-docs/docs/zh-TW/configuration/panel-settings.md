# 面板設定

面板「設定」頁集中管理執行時配置，修改即時生效並持久化到 SQLite 資料庫，無需編輯配置檔案或重啟服務。部分此前寫在 `config.yaml` 裡的配置（AI、Agent 令牌、監控認證）已遷移到這裡。

> 訪問「面板設定」需要 `settings.view` 許可權，儲存修改需要 `settings.edit` 許可權（預設 `admin` 角色具備）。

## 基礎設定

| 項 | 說明 |
|----|------|
| 面板埠 | 面板監聽埠（對應 `server.port`） |
| 面板域名 | 面板訪問域名，用於生成連結與反向代理場景 |
| 安全入口 | 隱藏路徑字首（對應 `server.secret_entry`），如 `/opsmini_panel`，留空不啟用 |
| 開啟 HTTPS | 是否啟用 HTTPS（推薦配合反向代理在邊緣終止 TLS） |

## 二步驗證（2FA）

- **全域性開關**：開啟後，所有已繫結驗證器的賬號登入時均需輸入 6 位動態碼
- 每個使用者可獨立繫結/解綁 TOTP 驗證器（Google Authenticator / 1Password / 各雲廠商驗證器 APP 相容）
- 丟失驗證器時，可登入伺服器用 `opsmini -reset-mfa <username>` 重置

詳見 [使用者與許可權../features/users-rbac.md#双因素验证-2fa。

## AI 大模型接入

統一在此配置內建 AI 助手的模型接入，替代原先的 `config.yaml` `ai` 段：

| 項 | 說明 |
|----|------|
| 啟用 AI 助手 | 總開關，關閉後 AI 相關功能不可用 |
| 模型服務商 | `openai` / `deepseek` / `qwen` / `ollama` |
| 模型 | 模型名，如 `gpt-4o`、`deepseek-chat`、`qwen-plus` |
| API Key | 服務商金鑰；留空可讀環境變數 `OPSMINI_AI_KEY` |
| API 地址 | OpenAI 相容介面 Base URL，如 `https://api.openai.com/v1` |
| 自然語言執行操作 | 是否允許 AI 執行運維操作 |
| 日誌智慧分析 | 是否啟用 AI 日誌分析 |
| 告警智慧診斷 | 是否啟用 AI 告警診斷 |

> 配置優先順序：環境變數 `OPSMINI_AI_KEY` > 面板設定 > 配置檔案殘留的 `ai.*`。詳見 [AI 助手../features/ai-assistant.md。

## API Token

為外部系統（監控平臺、自動化指令碼、第三方編排工具）生成訪問令牌：

- 生成的令牌用於呼叫 `/agent/v1` 介面，請求頭攜帶 `Authorization: Bearer <token>`
- 留空則**禁用** Agent API
- 支援「生成隨機」一鍵生成強隨機令牌；儲存後生效
- 洩露後應立即重新生成

命令白名單仍在 `config.yaml` 的 `agent.allowed_commands` 中維護。詳見 [Agent API../api/agent-api.md。

## 外觀與主題

- **自定義主色**：設定面板品牌主色（企業品牌色 / 個人偏好），預設 OpsMini 藍 `#4f6ef7`
- **選單顯示 / 語言**：面板介面語言（7 語言）與選單可見性

## 監控匯出（Prometheus）

控制 `/metrics` 端點（node_exporter 相容）的認證：

| 項 | 說明 |
|----|------|
| 啟用認證 | 是否為 `/metrics` 開啟 HTTP Basic 認證 |
| 使用者名稱 / 密碼 | Basic 認證憑據，使用者名稱留空即開放訪問 |

> 此處設定優先於 `config.yaml` 的 `metrics.user/password`。Prometheus 抓取時需配置對應 `basic_auth`。詳見 [Prometheus 指標../api/prometheus.md。

## 應用模版（軟體商店）

管理軟體商店的應用模版：

- **同步官方模版**：從 opsmini.com 官網應用商店一鍵同步官方應用模版
- **手動匯入/匯出**：離線環境可從官網下載模版後手動匯入，也可匯出本地模版備份
- **資料目錄**：應用資料持久化目錄（安裝指令碼透過 `DATA_DIR` 環境變數引用）

## 通知

配置告警通知渠道：

| 渠道 | 說明 |
|------|------|
| SMTP 郵件 | SMTP 伺服器 / 埠 / 發件人 / 收件人 / 使用者名稱 / 密碼 |
| 企業微信 | 群機器人 Webhook 位址 |
| 釘釘 | 群機器人 Webhook 位址 |
| 飛書 | 群機器人 Webhook 位址 |

告警觸發後按此處配置的渠道傳送通知。

## 日誌保留

- **面板日誌保留時間**：審計日誌與訪問日誌的保留天數（預設 7 天），過期自動清理
- 告警事件保留天數可另行設定（預設 30 天）

## 設定儲存

面板設定以 key-value 形式存於 SQLite 的 `settings` 表，執行時讀取。兩類敏感項由服務端保護：

- **只讀敏感**（`jwt_secret`、`metrics_pass`）：介面不返回給前端
- **防寫**（`jwt_secret`）：客戶端不可覆蓋，由服務端自動生成管理
