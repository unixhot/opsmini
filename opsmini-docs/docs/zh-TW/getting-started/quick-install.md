# 5 分鐘快速安裝

使用官方安裝指令碼 `install.sh`，在 Linux 主機上一鍵完成部署。

## 前提

- 目標主機：Linux `x86_64` 或 `aarch64`
- 擁有 `sudo` 許可權
- 可訪問外網（用於下載二進位制）或提前準備二進位制檔案

## 一鍵安裝

```bash
# 一鍵安裝（推薦，自動按架構從官網下載）
curl -fsSL https://opsmini.com/install.sh | sudo bash

# 方式一：从 URL 下载并安装（发布后使用 GitHub Release 地址）
sudo ./install.sh -u https://example.com/opsmini-v1.0.0-linux-amd64.tar.gz

# 方式二：指定本地二进制安装（默认安装到 /data/opsmini，端口 8888）
sudo ./install.sh -b ./opsmini

# 自定义目录与端口
sudo ./install.sh -b ./opsmini -d /data/opsmini -p 9999
```

## 安裝引數

| 引數 | 說明 | 預設值 |
|------|------|--------|
| `-d, --dir <path>` | 安裝目錄 | `/data/opsmini` |
| `-p, --port <port>` | 面板監聽埠 | `8888` |
| `-b, --binary <path>` | opsmini 二進位制路徑 | 自動查詢 |
| `-u, --url <url>` | 從 URL 下載（`.tar.gz` 或裸二進位制） | — |
| `-n, --no-systemd` | 不註冊 systemd 服務 | — |

## 安裝行為

1. 建立安裝目錄（預設 `/data/opsmini`）
2. 複製 / 下載二進位制到 `<目录>/opsmini`
3. 生成 `<目录>/config.yaml`（隨機 JWT secret、隨機 Agent token、埠、SQLite 路徑）
4. 生成 16 位隨機管理員密碼，寫入 `<目录>/.init_passwd`（許可權 600）
5. 註冊並啟動 systemd 服務 `opsmini.service`
6. 探測服務響應，列印訪問地址 / 使用者名稱 / 密碼

## 產物結構

```
/data/opsmini/
├── opsmini              # 二进制
├── config.yaml          # 配置（600）
├── opsmini.db           # SQLite 数据库（首次启动后生成）
└── .init_passwd         # 初始密码（600）
```

## 首次登入

啟動後日誌會列印類似：

```
initial admin account created: username=opsmini password=ggFKEX65ZVF6RNTm
```

- 訪問 `http://<host>:8888`
- 使用者名稱：`opsmini`
- 密碼：見啟動日誌或 `/data/opsmini/.init_passwd`

> ⚠️ 請立即記錄密碼並登入後修改。密碼僅首次安裝時生成一次。

## 下一步

- [首次登入與初始化first-steps.md
- [手動部署（systemd）../installation/manual-install.md
- [反向代理與 HTTPS../configuration/ports-proxy.md
