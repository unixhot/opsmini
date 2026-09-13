# 一鍵指令碼安裝

官方 `install.sh` 在 Linux 主機上一鍵完成部署。

## 快速開始

```bash
# 一鍵安裝（推薦，指令碼自動按架構從阿里雲 OSS 下載二進位制）
curl -fsSL https://opsmini.com/install.sh | sudo bash

# 指定本地二進位制安裝（預設安裝到 /data/opsmini，埠 8888）
sudo ./install.sh -b ./opsmini

# 自定義下載地址（支援裸二進位制或 .tar.gz）
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# 自定義目錄與埠
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## 引數

| 引數 | 說明 | 預設值 |
|------|------|--------|
| `-d, --dir <path>` | 安裝目錄 | `/data/opsmini` |
| `-p, --port <port>` | 面板監聽埠 | `8888` |
| `-b, --binary <path>` | opsmini 二進位制路徑 | 未指定時從 OSS 下載 |
| `-u, --url <url>` | 從 URL 下載（裸二進位制或 `.tar.gz`） | OSS 官方地址 |
| `-n, --no-systemd` | 不註冊 systemd 服務 | — |
| `-h, --help` | 幫助 | — |

## 安裝行為

1. 建立安裝目錄（預設 `/data/opsmini`）
2. 下載 / 複製二進位制到 `<目錄>/opsmini`（預設按架構從阿里雲 OSS 下載）
3. 生成 `<目錄>/config.yaml`（埠、SQLite 路徑、JWT 有效期、日誌級別與日誌檔案；JWT 金鑰與 Agent Token 首次啟動自動生成存庫）
4. 生成 16 位隨機管理員密碼，寫入 `<目錄>/.init_passwd`（許可權 600）
   - **全新安裝**：首次啟動時透過 `OPSMINI_INIT_PASSWORD` 環境變數注入
   - **重灌**（資料庫已存在）：自動用 `-reset-pass` 重置密碼，輸出的密碼即為生效密碼
5. 註冊並啟動 systemd 服務 `opsmini.service`
6. 探測服務響應，並列印訪問地址 / 使用者名稱 / 密碼 / 日誌檔案路徑

## 產物

```
/data/opsmini/
├── opsmini              # 二進位制
├── config.yaml          # 配置（600）
├── opsmini.db           # SQLite 資料庫（首次啟動後生成）
├── opsmini.log          # 執行日誌檔案
└── .init_passwd         # 初始密碼（600）
```

## 二進位制獲取

- **官方分發（阿里雲 OSS）**：`https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v<版本>-linux-<架構>`，指令碼預設從此處下載
- **本地開發**：專案根目錄 `make build-all` 產出 `dist/opsmini-<ver>-linux-amd64` / `-arm64`，指令碼會按架構自動查詢

## 相容性

- 僅支援 `x86_64` / `aarch64`
- 無 systemd 的環境用 `-n` 跳過服務註冊，改用手動啟動

## 賬號恢復 {: #account-recovery }

忘記密碼或丟失 MFA 驗證碼時，在伺服器上執行（先停服務，操作完再啟動）：

```bash
# 重置密碼
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-pass opsmini
systemctl start opsmini

# 清除 MFA 繫結
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-mfa opsmini
systemctl start opsmini
```

> 說明：`-reset-mfa` / `-reset-pass` 是賬號恢復子命令，直接操作 SQLite 資料庫後即退出，不會啟動 Web 服務，務必保證操作時服務處於停止狀態。
