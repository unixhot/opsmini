# 目錄結構

## 分層

每個模組內部嚴格三層：

```
Controller（HTTP handler）：参数解析校验、调用 Service、组装响应
    → Service（业务逻辑）：编排 Repository 与系统资源、事务边界
    → Repository（GORM）：纯数据访问，不写业务
```

## 目錄組織

```
opsmini/
├── cmd/
│   └── agent/main.go          # 主程序入口
├── internal/
│   ├── router/                # 路由注册、API 版本
│   ├── middleware/            # 认证、权限、审计、限流、安全入口
│   ├── api/v1/                # Controller（按模块分包）
│   ├── service/               # Service（按模块分包）
│   ├── repository/            # Repository（按模块分包）
│   ├── model/                 # GORM 数据模型
│   ├── config/                # 配置加载
│   └── pkg/                   # 通用工具（jwt/response/store）
├── web/                       # Vue3 前端（构建后 embed）
│   ├── index.html             # SPA 入口
│   └── static/                # echarts/vue/xterm 等静态库
├── configs/                   # 默认配置示例
├── docs/                      # 文档
└── install/                   # 一键安装脚本
```

## 關鍵模組

| 模組 | 職責 |
|------|------|
| `auth` | 登入 / 退出、會話、2FA、RBAC |
| `setting` | 面板配置、主題、選單顯隱、語言 |
| `dashboard` / `monitor` | 指標聚合、時序採集、告警規則 |
| `website` / `database` / `appstore` | 網站、資料庫、軟體商店 |
| `container` | Docker 容器 / 映象 / 磁碟區 / 網路 |
| `system` / `file` / `terminal` | 系統資源、檔案、Web 終端 |
| `cron` / `log` | 計劃任務、日誌 |
| `ai` | LLM 接入、上下文、NL 執行 |
| `security` | 基線、FIM、威脅、防火牆、登入安全 |
| `agent` | 對外 REST API（`/agent/v1`） |
