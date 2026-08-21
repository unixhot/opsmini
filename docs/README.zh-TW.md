<div align="center">

# OpsMini

**AI 驅動的主機維運面板**

[English](../README.md) · [简体中文](README.zh-CN.md) · [繁體中文](README.zh-TW.md) · [日本語](README.ja.md) · [한국어](README.ko.md) · [ไทย](README.th.md) · [Deutsch](README.de.md)

</div>

---

## 繁體中文

OpsMini 是一款 AI 驅動的主機維運面板（對標寶塔 / 1Panel），以內建 AI 大模型維運與對外標準 REST API 為核心差異化。

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
- [主機安全設計](docs/security-audit.md)

### 版權

OpsMini@2026 北京速云科技有限公司 (opsmini.com)
