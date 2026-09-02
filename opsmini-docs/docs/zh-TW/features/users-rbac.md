# 使用者與許可權

OpsMini 採用基於角色的訪問控制（RBAC），精細管理面板操作許可權。

## 使用者管理

- 建立 / 編輯 / 停用 / 刪除使用者
- 檢視最後登入時間
- 每個使用者可繫結獨立 MFA（TOTP）
- 個人設定頁支援修改暱稱、頭像、郵箱與密碼

## 角色與許可權

內建三種角色：

| 角色 | 許可權範圍 |
|------|----------|
| `admin` | 全部許可權（含使用者 / 角色 / 設定） |
| `operator` | 日常運維（應用、容器、檔案、終端、計劃任務等） |
| `readonly` | 只讀檢視 |

支援自定義角色，按**許可權點**精細分配，例如：

- `user.create` / `user.edit` / `user.delete`
- `website.create` / `website.edit` / `website.delete`
- `container.edit` / `container.delete`
- `security.scan` / `security.firewall` / `security.fim` / `security.threat`
- `settings.view` / `settings.edit`（面板設定）
- `apps.install`、`cron.create`、`database.create`、`file.write`、`mcp.manage`、`skill.manage`、`alert.manage` 等

## 雙因素驗證（2FA）

- **全域性開關**：面板「設定 → 二步驗證」開啟後，所有已繫結驗證器的賬號登入時均需輸入 6 位動態碼
- **每使用者繫結**：使用者在個人安全設定中生成二維碼，用 Google Authenticator / 1Password / 各雲廠商驗證器 APP 掃碼繫結
- **賬號恢復**：丟失驗證器時，可登入伺服器執行 `opsmini -reset-mfa <username>` 重置該使用者的 MFA 繫結

## 安全設計

- 密碼 bcrypt 雜湊儲存
- 短期 access JWT（15 分鐘）+ 可撤銷 refresh token（7 天）
- JWT 簽名金鑰首次啟動自動生成並持久化到資料庫，不落配置檔案
- 登入失敗限流與鎖定（防爆破）
- 關鍵操作寫審計日誌
- 忘記密碼可登入伺服器執行 `opsmini -reset-pass <username>` 重置
