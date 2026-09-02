# 一鍵指令碼安裝

官方 `opsmini-install.sh` 在 Linux 主機上一鍵完成部署。

## 快速開始

```bash
# 指定二进制安装（默认安装到 /data/opsmini，端口 8888）
sudo ./opsmini-install.sh -b ./opsmini

# 从 URL 下载并安装（支持 .tar.gz）
sudo ./opsmini-install.sh -u https://example.com/opsmini-v1.0.0-linux-amd64.tar.gz

# 自定义目录与端口
sudo ./opsmini-install.sh -b ./opsmini -d /opt/opsmini -p 9999
```

## 引數

| 引數 | 說明 | 預設值 |
|------|------|--------|
| `-d, --dir <path>` | 安裝目錄 | `/data/opsmini` |
| `-p, --port <port>` | 面板監聽埠 | `8888` |
| `-b, --binary <path>` | opsmini 二進位制路徑 | 自動查詢 |
| `-u, --url <url>` | 從 URL 下載（`.tar.gz` 或裸二進位制） | — |
| `-n, --no-systemd` | 不註冊 systemd 服務 | — |
| `-h, --help` | 幫助 | — |

## 安裝行為

1. 建立安裝目錄（預設 `/data/opsmini`）
2. 複製 / 下載二進位制到 `<目录>/opsmini`
3. 生成 `<目录>/config.yaml`（隨機 JWT secret、隨機 Agent token、埠、SQLite 路徑）
4. 生成 16 位隨機管理員密碼，寫入 `<目录>/.init_passwd`（許可權 600）
5. 註冊並啟動 systemd 服務 `opsmini.service`（首次啟動時透過 `OPSMINI_INIT_PASSWORD` 環境變數注入初始密碼，已有賬號時自動忽略）
6. 探測服務響應，並列印訪問地址 / 使用者名稱 / 密碼

## 產物

```
/data/opsmini/
├── opsmini              # 二进制
├── config.yaml          # 配置（600）
├── opsmini.db           # SQLite 数据库（首次启动后生成）
└── .init_passwd         # 初始密码（600，首次启动后不再变化）
```

## 二進位制獲取

- **本地開發**：專案根目錄 `make build-all` 產出 `dist/opsmini-<ver>-linux-amd64` / `-arm64`，指令碼會按架構自動查詢指令碼同目錄、`dist/`、上層 `dist/` 下的產物
- **釋出後**：用 `-u` 指向 GitHub Release 的 `.tar.gz` 地址

## 相容性

- 僅支援 `x86_64` / `aarch64`
- 無 systemd 的環境用 `-n` 跳過服務註冊，改用手動啟動

## 賬號恢復

忘記密碼或丟失 MFA 驗證碼時，在伺服器上執行（先停服務，操作完再啟動）：

```bash
# 重置密码
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-pass opsmini
systemctl start opsmini

# 清除 MFA 绑定
systemctl stop opsmini
/data/opsmini/opsmini -config /data/opsmini/config.yaml -reset-mfa opsmini
systemctl start opsmini
```

> 說明：`-reset-mfa` / `-reset-pass` 是賬號恢復子命令，直接操作 SQLite 資料庫後即退出，不會啟動 Web 服務，務必保證操作時服務處於停止狀態。
