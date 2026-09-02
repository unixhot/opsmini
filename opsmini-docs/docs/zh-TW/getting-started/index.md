# 快速開始

歡迎使用 OpsMini —— 一款輕量級 Linux 主機運維面板。本指南幫助你在最短時間內完成安裝、登入並瞭解核心功能。

## 從這裡開始

<div class="grid cards" markdown>

-   :material-rocket-launch: **[5 分鐘快速安裝quick-install.md**

    ---

    一行命令完成部署，首次啟動自動生成管理員賬號與隨機密碼。

-   :material-login: **[首次登入first-steps.md**

    ---

    登入面板、修改密碼、繫結雙因素驗證（MFA）。

-   :material-information-outline: **[產品介紹introduction.md**

    ---

    瞭解 OpsMini 的定位、核心特性與技術棧。

</div>

## 核心概念

| 概念 | 說明 |
|------|------|
| **單機面板** | 每臺主機獨立執行一個 OpsMini 例項，不依賴中心節點 |
| **單二進位制交付** | 前端 UI 透過 `go:embed` 嵌入，部署即複製一個可執行檔案 |
| **雙 API** | `/api/v1` 面向瀏覽器 UI（JWT + RBAC），`/agent/v1` 面向機器/第三方整合（Token + 命令白名單） |
| **AI 大模型運維** | 自然語言診斷、日誌分析、命令執行，接入 OpenAI / DeepSeek / Qwen / Ollama |

## 下一步

完成安裝後，建議按順序閱讀：

1. [安裝部署../installation/index.md — 系統要求、一鍵安裝、手動部署、反向代理
2. [配置指南../configuration/index.md — 配置檔案、資料儲存、HTTPS
3. [功能指南../features/index.md — 儀表盤、AI 助手、應用管理、主機安全等
4. [REST API../api/index.md — 對外整合介面
