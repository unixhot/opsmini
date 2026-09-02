# 面板 API（/api/v1）

面板 API 面向瀏覽器 UI，使用使用者會話 JWT + RBAC 認證。

## 認證

- 登入介面 `POST /auth/login` 返回 `access` + `refresh` token
- 後續請求攜帶 `Authorization: Bearer <access_token>`
- access 過期後用 refresh 換新
- 登入限流（防爆破）；開啟 2FA 後登入需先 `POST /auth/mfa/verify` 校驗動態碼

## 統一響應

```json
{ "code": 0, "message": "ok", "data": { } }
```

`code` 非 0 為業務錯誤。

## 端點一覽

### 認證與使用者

| 方法 | 路徑 | 說明 |
|------|------|------|
| POST | `/auth/login` | 登入（限流） |
| POST | `/auth/refresh` | 重新整理 token |
| POST | `/auth/logout` | 退出 |
| POST | `/auth/mfa/verify` | 登入時校驗 MFA 動態碼 |
| GET | `/auth/mfa/status` | 查詢當前使用者 MFA 繫結狀態 |
| POST | `/auth/mfa/setup` | 生成 MFA 繫結二維碼/金鑰 |
| POST | `/auth/mfa/enable` | 校驗並啟用 MFA |
| POST | `/auth/mfa/disable` | 解綁 MFA |
| GET | `/profile` | 個人資料（暱稱/頭像/郵箱） |
| PUT | `/profile` | 修改個人資料 |
| POST | `/profile/password` | 修改密碼 |
| GET/POST/PUT/DELETE | `/users` | 使用者 CRUD |
| GET/POST/PUT/DELETE | `/roles` | 角色 CRUD |
| GET | `/roles/groups` | 許可權分組 |
| GET | `/permissions` | 當前使用者許可權 |

### 面板設定

| 方法 | 路徑 | 說明 |
|------|------|------|
| GET | `/settings` | 讀取全部非敏感設定（`settings.view`） |
| PUT | `/settings` | 批次更新設定（`settings.edit`） |

> 敏感項（`jwt_secret`、`metrics_pass`）不返回；防寫項（`jwt_secret`）不可覆蓋。詳見 [面板設定../configuration/panel-settings.md。

### 監控與告警

| 方法 | 路徑 | 說明 |
|------|------|------|
| GET | `/dashboard/overview` | 儀表盤概覽 |
| GET | `/dashboard/metrics` | 儀表盤指標 |
| GET | `/system/monitor` | 監控摘要 |
| CRUD | `/alert-rules` | 告警規則 |
| GET | `/alert-events` | 告警事件 |

### 通知

| 方法 | 路徑 | 說明 |
|------|------|------|
| GET | `/notifications` | 通知列表 |
| GET | `/notifications/unread-count` | 未讀數 |
| PUT | `/notifications/read-all` | 全部已讀 |
| PUT | `/notifications/:id/read` | 標記已讀 |
| DELETE | `/notifications/:id` / `/notifications` | 刪除單條 / 清空 |

### 資源管理

| 方法 | 路徑 | 說明 |
|------|------|------|
| CRUD | `/websites` | 網站 |
| CRUD | `/databases` | 資料庫 |
| GET/POST | `/apps` `/app-categories` | 軟體商店與分類 |
| GET/POST | `/containers` `/images` `/volumes` `/networks` | 容器四類 |
| GET/POST | `/files` | 檔案 |
| CRUD | `/cron-jobs` | 計劃任務 |
| GET | `/logs` `/logs/tail` | 日誌 |

### 系統與安全

| 方法 | 路徑 | 說明 |
|------|------|------|
| GET | `/system/info` `/processes` `/ports` `/disks` `/network` `/users` `/groups` `/firewall` | 系統資源 |
| GET/POST | `/security/...` | 主機安全（基線 / FIM / 威脅 / 防火牆 / 登入安全） |
| GET | `/audit-logs` `/access-logs` | 審計 / 訪問日誌 |

### AI 與整合

| 方法 | 路徑 | 說明 |
|------|------|------|
| POST | `/ai/chat` | AI 對話（函式呼叫） |
| POST | `/ai/chat/stream` | AI 對話（流式 SSE） |
| GET/POST/PUT/DELETE | `/mcp` | MCP 配置 |
| GET/POST/DELETE | `/skills` 等 | AI 技能（SkillHub 檢索/安裝/上傳） |

### 終端

| 方法 | 路徑 | 說明 |
|------|------|------|
| GET | `/terminal` | WebSocket 終端（query token 認證） |
| GET | `/containers/:id/exec` | 容器 WebSocket 終端 |

## 說明

- 所有寫操作受 RBAC 許可權點控制（如 `website.create`、`container.edit`、`settings.edit`）
- 關鍵操作寫審計日誌，登入態請求寫訪問日誌
