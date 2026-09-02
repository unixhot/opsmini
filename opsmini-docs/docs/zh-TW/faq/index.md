# 常見問題

## 安裝與啟動

**Q：`go mod tidy` 卡住或報 Bad Gateway？**

國內網路下 `proxy.golang.org` 可能被牆，執行 `export GOPROXY=https://goproxy.cn,direct`。

**Q：提示 Go 版本過低？**

OpsMini 要求 Go 1.25+。下載官方預編譯二進位制並加入 PATH。

**Q：訪問 `/` 返回 301 或空白？**

前端靜態資源已透過 `go:embed` 嵌入，請使用 `make build` 產物，勿手動拆分前端目錄。

## 賬號與安全

**Q：忘記管理員密碼？**

伺服器上執行 `opsmini -config <path> -reset-pass opsmini` 重置為隨機密碼（詳見[安裝部署../installation/index.md）。

**Q：丟失 MFA 雙因素驗證碼？**

執行 `opsmini -config <path> -reset-mfa opsmini` 清除 MFA 繫結後重新登入繫結。

## 部署與運維

**Q：如何升級版本？**

備份 `opsmini.db` → 替換二進位制 → 重啟服務。SQLite 由 GORM 自動遷移，通常無需手動改表。

**Q：Web 終端在反向代理後連不上？**

需在 Nginx 中啟用 WebSocket 升級頭（`proxy_http_version 1.1` + `Upgrade`/`Connection`），詳見[埠與反向代理../configuration/ports-proxy.md。

**Q：如何讓 Prometheus 監控本機？**

在配置中啟用 `metrics`，Prometheus 直接 scrape `http://<host>:8888/metrics`。
