<div align="center">

# OpsMini

**主機上的 AI 維運 Agent**

[English](README.en.md) · [简体中文](../README.md) · [繁體中文](README.zh-TW.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [ไทย](README.th.md) · [Deutsch](README.de.md)

</div>

---

## 繁體中文

OpsMini 是一台 Linux 主機上的常駐 AI 維運 Agent：自己採集指標、自己判斷異常、自己動手處理，遇到高風險動作停下來等你點頭。Web 介面只是它的觀察窗——你可以什麼都不點，它照樣每天把活幹完並留下報告。整個 Agent 是一個數十 MB 的靜態二進位，資料全部留在本機。

### 特性

- **AI 大模型運維** — 自然語言診斷、日誌分析、指令執行
- **標準 REST API** — `/api/v1`（瀏覽器 UI）與 `/agent/v1`（機器對機器）
- **Docker 管理** — 容器、映像、磁碟區、網路
- **Web 終端機** — WebSocket + pty 實作的類 SSH 互動式終端機
- **7 語言 i18n** — 簡/繁中文、英文、日文、韓文、泰文、德文
- **單一執行檔交付** — 前端透過 `go:embed` 嵌入，零執行時期相依

### 技術棧

Go · Gin · GORM · SQLite（純 Go）· Vue 3 · ECharts

### 快速開始

```bash
make build          # 編譯目前平台
make build-all      # 交叉編譯 Linux amd64/arm64

./dist/opsmini -config configs/config.yaml
# 開啟 http://localhost:8888（預設帳號 opsmini，密碼見首次啟動日誌）
```

### 文件

- [開發者手冊](docs/developer-guide.zh-CN.md)
- [後端架構設計](docs/backend-architecture.md)
- [建置與部署手冊](docs/build-and-deploy.md)
- [主機安全設計](host-security-design.md)
- [安全審查報告](security-audit.md)

### 版權

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
