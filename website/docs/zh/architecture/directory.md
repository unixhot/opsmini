# 目录结构

## 分层

每个模块内部严格三层：

```
Controller（HTTP handler）：参数解析校验、调用 Service、组装响应
    → Service（业务逻辑）：编排 Repository 与系统资源、事务边界
    → Repository（GORM）：纯数据访问，不写业务
```

## 目录组织

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

## 关键模块

| 模块 | 职责 |
|------|------|
| `auth` | 登录 / 退出、会话、2FA、RBAC |
| `setting` | 面板配置、主题、菜单显隐、语言 |
| `dashboard` / `monitor` | 指标聚合、时序采集、告警规则 |
| `website` / `database` / `appstore` | 网站、数据库、软件商店 |
| `container` | Docker 容器 / 镜像 / 卷 / 网络 |
| `system` / `file` / `terminal` | 系统资源、文件、Web 终端 |
| `cron` / `log` | 计划任务、日志 |
| `ai` | LLM 接入、上下文、NL 执行 |
| `security` | 基线、FIM、威胁、防火墙、登录安全 |
| `agent` | 对外 REST API（`/agent/v1`） |
