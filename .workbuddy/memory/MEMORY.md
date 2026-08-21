# OpsMini 项目长期事实

## 项目定位
- OpsMini = 轻量级单机运维面板（对标宝塔/1Panel）。
- 两大差异化：① 内置 AI 大模型做系统管理；② Agent/Proxy 多机统一管理。
- Agent/Proxy 通信模型（2026-08-13 确定）：**Agent 暴露标准 REST API（`/agent/v1`），Proxy 主动拉取管理**（拉取模型，类 Ansible/Prometheus），而非 salt minion 的出站长连接推送。面板 API `/api/v1`（人用）与 Agent API `/agent/v1`（机用）分离。

## 技术栈（已确定）
- 前端：Vue3（单文件原型已成型，见 `index.html`，最终将工程化为 `web/` 目录 + 构建产物 embed 进 Go 二进制）。
- 后端：Golang（Gin + GORM + modernc.org/sqlite 纯Go驱动，M1 已跑通）。
- 数据库：SQLite。
- 交付形态：单二进制（opsmini）+ 可选 Proxy 中心。
- **发布架构**：需同时支持 x86_64（amd64）与 arm64（用户开发机是 Apple Silicon）。纯 Go 驱动无 CGO → `CGO_ENABLED=0` 一键交叉编译静态二进制。`Makefile` 已支持 `make build-all` 产出 5 平台（linux/amd64、linux/arm64、darwin/amd64、darwin/arm64、windows/amd64）。

## 关键依赖（go.mod）
- 框架/数据：gin、gorm、glebarez/sqlite（纯Go SQLite）
- 鉴权：golang-jwt/jwt/v5、bcrypt（x/crypto）
- 系统指标：shirou/gopsutil/v4
- 调度：robfig/cron/v3
- 终端：gorilla/websocket + creack/pty
- 容器：moby/moby/client（注意：**不是** docker/docker/client，模块已迁移到 moby；api 类型在独立的 moby/moby/api 模块）

## 关键交付物
- `web/index.html`：高保真可交互原型（7 语言 i18n、主题系统、登录、容器管理、计划任务、弹窗表单等）。**已通过 `web/embed.go` 的 go:embed 嵌入二进制**，单文件交付。
- `docs/backend-architecture.md`：后端架构设计文档。
- `web/static/`：前端本地离线依赖（vue.global.js + echarts.min.js）。注意：原叫 `vendor/`（因与 Go 模块 vendor 目录约定冲突改名 `static/`），后随前端一起移入 `web/` 目录。
- `cmd/`、`internal/`：Go 后端源码，**M1~M7 七个里程碑已全部完成**：
  - M1 骨架（登录/用户/RBAC）
  - M2 系统能力（gopsutil 指标/进程/端口/磁盘/网络/仪表盘/监控）
  - M3 资源管理（网站/数据库/计划任务/文件/终端）
  - M4 容器（Docker SDK）
  - M5 AI（LLM 适配层）
  - M6 Agent/Proxy（Agent REST API + token 认证）
  - M7 打磨（审计日志/登录限流/单元测试）

## 开发环境（本机）
- Go 工具链：`~/go-sdk125/go`（go1.25.0，官方 tarball 解压；brew 安装太慢/无 bottle）。用 `export PATH=~/go-sdk125/go/bin:$PATH`。
- GOPROXY：`https://goproxy.cn,direct`（proxy.golang.org 在本机网络被墙，返回 Bad Gateway）。
- 构建：`go build -o opsmini ./cmd/agent`；运行：`./opsmini -config configs/config.yaml`（默认端口 8888，默认账号 admin/opsmini）；多架构 `make build-all`。

## 迭代习惯
- 每轮改动后更新 `.workbuddy/memory/YYYY-MM-DD.md`。
- 无头 Chrome 渲染截图验证 UI（`--headless=new --no-sandbox`）。
- 注意：JS 对象字面量重复键会静默覆盖（Vue 不报错），测试临时改 data 时别新增同名键。
- 后端实测流程：编译 → 后台启动 `run_in_background` → curl 登录拿 token → 测接口 → TaskStop 停服。
- **Git 提交**：项目已接入 git 仓库（远程 `https://codeup.aliyun.com/womaiyun/womaiyun/opsmini.git`，分支 `main`）。每轮开发完成（部署验证通过后）需 `git add -A && git commit -m "..." && git push` 推送代码。
