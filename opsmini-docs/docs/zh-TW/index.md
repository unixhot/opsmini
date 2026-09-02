---
title: OpsMini — 轻量级 Linux 主机运维面板
hide:
  - navigation
  - toc
---

<div class="home-hero" markdown>

# OpsMini

**輕量級 Linux 主機運維面板** — 內建 AI 大模型運維，標準 REST API 對外整合

對標寶塔 / 1Panel，讓單機運維更簡單、更智慧、更可被整合。

<div class="home-cta" markdown>
[快速安裝getting-started/quick-install.md{ .md-button .md-button--primary }
[檢視檔案getting-started/index.md{ .md-button }
[:fontawesome-brands-github: GitHubhttps://github.com/unixhot/opsmini{ .md-button }
</div>

</div>

---

## 兩大核心差異化

<div class="grid cards" markdown>

-   :material-robot-outline: **AI 大模型運維**

    ---

    自然語言診斷故障、分析日誌、執行運維命令。接入 OpenAI / DeepSeek / Qwen / Ollama，讓 AI 成為你的運維副駕。

-   :material-api: **標準 REST API**

    ---

    內建 `/api/v1`（面板 API）與 `/agent/v1`（機器對機器 API），任何監控平臺、自動化指令碼、編排工具都能直接整合呼叫。

</div>

---

## 特性一覽

<div class="grid cards" markdown>

-   :material-view-dashboard-outline: **儀表盤與監控**

    ---

    實時 CPU / 記憶體 / 磁碟 / 網路時序，告警規則與告警事件閉環。

-   :material-docker: **容器管理**

    ---

    Docker 容器、映象、磁碟區、網路四類資源全託管，另有軟體商店一鍵安裝常用應用。

-   :material-console: **Web 終端**

    ---

    WebSocket + pty 實現的類 SSH 互動終端，瀏覽器裡直接操作伺服器。

-   :material-shield-check-outline: **主機安全**

    ---

    基線檢查、檔案完整性監控（FIM）、威脅檢測、防火牆、登入安全，安全態勢一屏掌握。

-   :material-folder-outline: **檔案與網站**

    ---

    檔案瀏覽 / 上傳 / 編輯，Nginx 網站、資料庫、SSL 證書一站式管理。

-   :material-translate: **7 語言 · 單二進位制**

    ---

    簡/繁中文、英、日、韓、泰、德 7 語言介面；前端 `go:embed` 嵌入，約 30MB 靜態連結，零執行時依賴。

</div>

---

## 一鍵安裝

<div class="home-section" markdown>

### 5 分鐘上手

```bash
# Linux x86_64 / aarch64，一键脚本安装到 /data/opsmini，端口 8888
curl -fsSL https://opsmini.com/install.sh | sudo bash
```

首次啟動自動生成管理員賬號 `opsmini` 與隨機密碼（見啟動日誌），開啟 `http://<host>:8888` 即可。

[檢視完整安裝檔案 →installation/index.md{ .md-button }

</div>

---

<div class="home-section" markdown>

## 立即開始

一行命令部署，AI 賦能運維，標準 API 打通整合。

[開始安裝getting-started/quick-install.md{ .md-button .md-button--primary }
[探索檔案getting-started/index.md{ .md-button }

</div>
