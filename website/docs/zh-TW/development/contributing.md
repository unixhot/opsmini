# 貢獻指南

感謝你對 OpsMini 的關注與貢獻！

## 參與方式

- **報告 Bug**：在 GitHub Issues 提交，附上覆現步驟、環境資訊、日誌
- **功能建議**：在 Issues 中描述使用場景與期望
- **提交程式碼**：Fork 倉庫 → 新建分支 → 提交 → 發起 Pull Request
- **完善檔案**：修正檔案錯誤、補充使用示例

## 提交規範

- 一個 PR 聚焦一個改動，避免大而全
- 遵循 Go 官方程式碼風格（`gofmt` / `go vet`）
- 新功能補充測試用例
- 提交資訊清晰描述「做了什麼」「為什麼」

## 目錄約定

- 分層：`api/v1`（Controller）→ `service`（業務）→ `repository`（資料）
- 敏感欄位（金鑰 / Token）加密落庫
- 新介面遵循統一響應 `{ code, message, data }`

## 協議

專案採用 Apache License 2.0，貢獻程式碼即視為同意按該協議授權。
