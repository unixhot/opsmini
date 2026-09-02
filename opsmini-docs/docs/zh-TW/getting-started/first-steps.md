# 首次登入與初始化

完成安裝後，按以下步驟完成首次登入與安全初始化。

## 1. 獲取初始密碼

啟動日誌會列印初始賬號資訊：

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

密碼也可在安裝目錄的 `.init_passwd`（許可權 600）中檢視。

## 2. 登入面板

1. 瀏覽器開啟 `http://<host>:8888`
2. 輸入使用者名稱 `opsmini` 與初始密碼
3. 登入成功後進入儀表盤

> ⚠️ 生產環境務必透過 [反向代理 + HTTPS../configuration/https.md 暴露面板，避免明文傳輸。

## 3. 修改密碼

1. 進入「個人設定」
2. 在「修改密碼」中設定新密碼並儲存

忘記密碼時，可登入伺服器執行 `opsmini -reset-pass <username>` 重置。

## 4. 繫結雙因素驗證（推薦）

1. 在面板「設定 → 二步驗證」開啟全域性 2FA
2. 進入個人安全設定，掃碼繫結 TOTP 驗證器（Google Authenticator / 1Password 等）
3. 輸入動態碼完成繫結

繫結後，每次登入需額外輸入 6 位動態碼，大幅提升賬號安全性。丟失驗證器可用 `opsmini -reset-mfa <username>` 恢復。

## 5. 配置安全入口（可選）

在面板「設定 → 基礎設定」中設定安全入口字首（或直接改 `config.yaml` 的 `server.secret_entry`）：

```yaml
server:
  secret_entry: "/opsmini_panel"
```

配置後面板地址變為 `http://<host>:8888/opsmini_panel`，可配合反向代理隱藏真實入口，防埠掃描。

## 6. 下一步

- [配置 AI 助手../configuration/panel-settings.md#ai-大模型接入
- [啟用對外 REST API../api/agent-api.md
- [探索功能../features/index.md
