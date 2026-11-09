# 外掛系統

核心只做「所有主機都需要的事」：採集、審計、Agent 執行時、護欄。業務能力（某個中介軟體的指標、某類日誌的解析、某個平臺的動作對接）用**外掛**擴充套件。

這樣做的目的不是追求架構美觀，而是避免兩件具體的事：**二進位制無限膨脹**，以及**為了一個新能力就得發一個版本**。

## 外掛是獨立程序，不是 Go plugin

- Go 的 `plugin` 包要求編譯期完全一致的構建環境，跨版本必然崩，直接排除
- 外掛是**獨立可執行檔案**，面板以子程序方式啟動它，透過 Unix socket 用**行分隔 JSON** 通訊
- 外掛崩潰不影響面板：程序退出會被記錄、按重啟策略拉起、連續失敗標記為 failed 並停止重試

## 包格式

```
foo-collector-1.2.0-linux-amd64.tar.gz
├── plugin.yaml          # 清單
├── bin/plugin           # 入口（可執行）
├── SIGNATURE            # ed25519 簽名（對整包 sha256 簽名）
└── README.md            # 可選
```

清單示例：

```yaml
apiVersion: opsmini.plugin/v1
id: foo-collector              # 全域性唯一，^[a-z0-9][a-z0-9-]{2,40}$
name: Foo 採集器
version: 1.2.0                 # semver
vendor: 速雲科技
description: 採集 Foo 服務的佇列深度與消費速率
kind: collector                # collector | action | panel
entry: bin/plugin              # 相對包內路徑
platform: [linux/amd64, linux/arm64]

capabilities:                  # 能力宣告，同時作為審計依據
  - metrics
  - read:/etc/foo
  - exec:systemctl
  - net:outbound

runtime:
  memoryLimitMB: 128
  cpuQuotaPercent: 20
  restartPolicy: on-failure    # always | on-failure | never

requires:
  opsmini: ">=1.4.0"           # 主機 Agent 版本要求
```

安裝時的校驗順序（任一失敗即拒絕，不裝「半截外掛」）：

1. 包 sha256（服務端計算，可與上傳者宣告的交叉校驗）
2. ed25519 簽名
3. `apiVersion` 相容性
4. `platform` 是否匹配當前主機
5. 清單欄位合法性、版本要求

## 三條通道

外掛與核心只透過 socket 說四種訊息，全部是單行 JSON：

| 訊息 | 方向 | 用途 |
|---|---|---|
| `hello` | 外掛 → 面板 | 啟動握手，宣告自己是誰、版本、能力 |
| `metric` | 外掛 → 面板 | 上報指標（進入監控與告警規則） |
| `event` | 外掛 → 面板 | 上報事件（可轉成訊號進入事件中心） |
| `heartbeat` | 外掛 → 面板 | 存活與自檢狀態 |

外掛不直接碰資料庫、不直接呼叫面板介面、不繼承面板的會話。

## 生命週期

```
安裝（上傳包 → 校驗 → 解包到 releases/<version>/ → current 軟鏈指向它）
   ↓
啟用 → 啟動程序 → 等 hello 握手 → 進入 running
   ↓
執行 → 崩潰/退出 → 按 restartPolicy 處理，記重啟次數與最後一次錯誤
   ↓
停用 → SIGTERM → 等待退出 → 狀態 stopped
   ↓
升級 → 新版本解包 → current 軟鏈切換 → 重啟（舊版本目錄保留，可回退）
   ↓
刪除 → 停程序 → 移除程式檔案（資料目錄保留）
```

## 許可權邊界：說清楚哪些做到了

這一節必須誠實，否則「外掛安全」會變成一句空話。

**做到了：**

- 外掛是獨立程序，崩潰不會拖垮面板
- 安裝必須驗籤（除非顯式開啟 unsigned 模式），且校驗版本、平臺、清單
- 安裝、啟用、停用、刪除全部記審計日誌
- 外掛透過 socket 通訊，不能直接操作面板資料庫

**當前版本沒做到（不要假設有）：**

- **沒有切換到獨立系統使用者**：外掛以面板程序的使用者執行，通常是 `root`
- **沒有 cgroup 資源限制**：清單裡的 `memoryLimitMB` / `cpuQuotaPercent` 目前是宣告，未強制
- **`capabilities` 未強制**：它是宣告與審計依據，不是核心級白名單
- **不能審計外掛內部的系統呼叫**

所以結論是：**簽名證明「來源可信」，不證明「行為安全」**。只安裝你信任來源的外掛；第三方外掛請自行審計程式碼。

## 安裝前提（當前版本需要注意）

安裝時會依次檢查：`sha256` → 簽名 → 清單。預設配置下**沒有內建公鑰，也沒有開啟 unsigned 模式**，因此直接上傳會被拒絕，報「未配置外掛公鑰且未開啟 unsigned 模式」。

兩種解決方式，任選一種（寫進設定後**需要重啟面板**才生效，因為公鑰在啟動時讀取）：

```bash
# 方式一：配置 ed25519 公鑰（推薦，正式環境）
curl -X POST http://127.0.0.1:8888/api/v1/settings \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"plugin_public_key":"<hex 編碼的 32 位元組公鑰>"}'

# 方式二：開發/內網環境臨時允許未簽名包
curl -X POST http://127.0.0.1:8888/api/v1/settings \
  -H "Authorization: Bearer <token>" -H "Content-Type: application/json" \
  -d '{"plugin_allow_unsigned":"true"}'
```

改完 `systemctl restart opsmini`，再回到「面板設定 → 外掛」上傳。

> 當前版本面板還沒有提供這兩個設定項的輸入介面，只能透過上面的介面或直接改設定表。下一版會補。

## 面板操作

**面板設定 → 外掛**：

| 操作 | 說明 |
|---|---|
| 上傳安裝 | 選擇 `*.tar.gz` 包，最大 64MB |
| 啟用 / 停用 | 立即生效，不需要重啟面板 |
| 重啟 | 單獨重啟某個外掛（例如改了它的配置） |
| 刪除 | 移除程式檔案，資料目錄保留 |
| 事件 | 檢視該外掛最近上報的指標與事件 |

列表裡能看到每個外掛的：名稱、版本、型別、狀態（running / stopped / failed / installed）、重啟次數、最後一次錯誤。

## 現狀說明

機制是完整的，**生態目前是空的**——v1.4.0 沒有附帶官方外掛包。這意味著裝完 OpsMini 後，外掛頁是空的，這是預期狀態而不是故障。

規劃中的官方外掛方向（未釋出）：安全基線擴充套件巡檢、特定中介軟體指標採集、日誌解析規則包、第三方平臺動作對接。

## 與其他模組的關係

| 模組 | 關係 |
|---|---|
| [自主 Agent（事件中心）](autonomous-agent.md) | 外掛上報的 `event` 可轉成訊號進入事件中心，參與聚合與處置 |
| [儀表盤與監控](dashboard-monitoring.md) | 外掛上報的 `metric` 可被告警規則使用 |
| [主機安全](security.md) | 官方安全外掛是主機安全的擴充套件方向 |
