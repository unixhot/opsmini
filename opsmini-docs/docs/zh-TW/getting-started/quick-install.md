# 5 分鐘快速安裝

使用官方安裝指令碼 `install.sh`，在 Linux 主機上一鍵完成部署。

## 前提

- 目標主機：Linux `x86_64` 或 `aarch64`
- 擁有 `sudo` 許可權
- 可訪問外網（用於下載二進位制）或提前準備二進位制檔案

## 一鍵安裝

```bash
# 一鍵安裝（推薦，指令碼自動按架構從阿里雲 OSS 下載二進位制）
curl -fsSL https://opsmini.com/install.sh | sudo bash

# 自定義下載地址
sudo ./install.sh -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64

# 指定本地二進位制安裝（預設安裝到 /data/opsmini，埠 8888）
sudo ./install.sh -b ./opsmini

# 自定義目錄與埠
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## 安裝引數

| 引數 | 說明 | 預設值 |
|------|------|--------|
| `-d, --dir <path>` | 安裝目錄 | `/data/opsmini` |
| `-p, --port <port>` | 面板監聽埠 | `8888` |
| `-b, --binary <path>` | opsmini 二進位制路徑 | 未指定時從 OSS 下載 |
| `-u, --url <url>` | 從 URL 下載（`.tar.gz` 或裸二進位制） | OSS 官方地址 |
| `-n, --no-systemd` | 不註冊 systemd 服務 | — |

## 安裝行為

1. 建立安裝目錄（預設 `/data/opsmini`）
2. 下載 / 複製二進位制到 `<目錄>/opsmini`（預設按架構從阿里雲 OSS 下載）
3. 生成 `<目錄>/config.yaml`（埠、SQLite 路徑、JWT 有效期、日誌級別與日誌檔案；JWT 金鑰與 Agent Token 首次啟動自動生成存庫）
4. 生成 16 位隨機管理員密碼，寫入 `<目錄>/.init_passwd`（許可權 600）
5. 註冊並啟動 systemd 服務 `opsmini.service`
6. 探測服務響應，列印訪問地址 / 使用者名稱 / 密碼 / 日誌檔案路徑

## 產物結構

```
/data/opsmini/
├── opsmini              # 二進位制
├── config.yaml          # 配置（600）
├── opsmini.db           # SQLite 資料庫（首次啟動後生成）
├── opsmini.log          # 執行日誌檔案
└── .init_passwd         # 初始密碼（600）
```

## 首次登入

- 訪問 `http://<host>:8888`
- 使用者名稱：`opsmini`
- 密碼：安裝指令碼列印的密碼，或檢視 `/data/opsmini/.init_passwd`

> ⚠️ 請立即記錄密碼並登入後修改。重灌時密碼會自動重置並重新列印。

## 下一步

- [首次登入與初始化](first-steps.md)
- [手動部署（systemd）](../installation/manual-install.md)
- [反向代理與 HTTPS](../configuration/ports-proxy.md)
