# 資料儲存

OpsMini 使用嵌入式 SQLite 儲存全部業務資料（使用者、任務、告警、網站、審計日誌等）。

## 資料檔案

- 預設路徑：`opsmini.db`（與配置檔案同目錄，或 `config.yaml` 中 `database.path` 指定）
- 首次啟動自動建立並建表（GORM 自動遷移）
- 敏感欄位（金鑰 / Token / TOTP secret）使用 AES-GCM 加密後落庫

## 備份

SQLite 是單檔案，直接複製即可備份：

```bash
# 建议先停服务再备份，保证一致性
sudo systemctl stop opsmini
cp /data/opsmini/opsmini.db /backup/opsmini.db.$(date +%s)
sudo systemctl start opsmini
```

## 恢復

```bash
sudo systemctl stop opsmini
cp /backup/opsmini.db.<ts> /data/opsmini/opsmini.db
sudo systemctl start opsmini
```

## 儲存內容概覽

| 資料 | 表 | 說明 |
|------|-----|------|
| 使用者與角色 | `users` / `roles` | 賬號、密碼雜湊、RBAC 角色與許可權 |
| 會話 | `sessions` | refresh token 與有效期 |
| 面板配置 | `settings` | 主題 / 語言 / 選單顯隱等 KV |
| 網站 / 資料庫 | `websites` / `databases` | 站點與資料庫例項記錄 |
| 計劃任務 | `cron_jobs` | 面板計劃任務 |
| 告警規則 / 事件 | `alert_rules` / `alert_events` | 監控告警 |
| 審計 / 訪問日誌 | `audit_logs` / `access_logs` | 操作與訪問記錄（預設保留 7 天） |

## 資料清理

- 審計日誌與訪問日誌預設保留 7 天，每小時自動清理，可在面板設定中調整保留天數
- 告警事件按保留時間自動清理（預設 30 天）
