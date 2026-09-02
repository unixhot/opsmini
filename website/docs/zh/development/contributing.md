# 贡献指南

感谢你对 OpsMini 的关注与贡献！

## 参与方式

- **报告 Bug**：在 GitHub Issues 提交，附上复现步骤、环境信息、日志
- **功能建议**：在 Issues 中描述使用场景与期望
- **提交代码**：Fork 仓库 → 新建分支 → 提交 → 发起 Pull Request
- **完善文档**：修正文档错误、补充使用示例

## 提交规范

- 一个 PR 聚焦一个改动，避免大而全
- 遵循 Go 官方代码风格（`gofmt` / `go vet`）
- 新功能补充测试用例
- 提交信息清晰描述「做了什么」「为什么」

## 目录约定

- 分层：`api/v1`（Controller）→ `service`（业务）→ `repository`（数据）
- 敏感字段（密钥 / Token）加密落库
- 新接口遵循统一响应 `{ code, message, data }`

## 协议

项目采用 Apache License 2.0，贡献代码即视为同意按该协议授权。
