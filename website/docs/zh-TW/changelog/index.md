# 更新日誌

各版本變更記錄。

## v1.0.25（當前）

近期主要更新：

- **配置精簡**：JWT 簽名金鑰首次啟動自動生成並持久化到資料庫，不再手動配置、不落配置檔案
- **AI 配置面板化**：AI 大模型接入（模型 / API Key / Base URL / 開關）遷移到面板「設定 → AI 大模型接入」頁
- **Agent Token 面板化**：`/agent/v1` 認證令牌遷移到面板「設定 → API Token」頁，留空禁用
- **新增雙因素驗證（2FA）**：全域性開關 + 每使用者 TOTP 繫結，支援 `-reset-mfa` 賬號恢復
- **新增賬號恢復**：`-reset-pass` 重置使用者密碼為隨機強密碼
- **監控匯出**：Prometheus `/metrics` 認證改為 HTTP Basic auth，支援面板設定覆蓋
- **主機安全模組**：基線檢查、檔案完整性監控（FIM）、威脅檢測、防火牆、登入安全
- **軟體商店**：應用分類 + 一鍵安裝 + 官方模版同步
- **通知管理**：SMTP / SNMP 通知渠道
- **AI 技能（Skill Hub）**：技能檢索、安裝、上傳
- **日誌保留**：審計 / 訪問日誌保留天數可配置
- **7 語言 i18n** 全覆蓋，品牌視覺升級（藍色 logo、側邊欄配色）

## v1.0.0

- 首個可執行的最小面板：登入 / 使用者 / RBAC / 儀表盤 / 監控
- 單二進位制交付，前端 `go:embed` 嵌入
- 雙 API：`/api/v1` 與 `/agent/v1`

> 完整版本歷史以倉庫 Release 為準：[GitHub Releaseshttps://github.com/unixhot/opsmini/releases
