# OpsMini 安全审查报告

- **审查日期**：2026-08-17
- **审查版本**：v1.5.7（修复后发布 v1.5.8）
- **审查范围**：Go 后端（认证/授权/命令执行/文件操作/Web 安全）与 Vue3 前端

---

## 一、发现的问题总览

| 级别 | 问题 | 状态 |
|------|------|------|
| 🔴 严重 | Agent 命令白名单可被 shell 元字符绕过，任意命令执行 | ✅ 已修复 |
| 🔴 严重 | JWT Secret 默认弱密钥 `"change-me"`，可伪造任意用户 token | ✅ 已修复 |
| 🟠 高危 | AI 聊天用 `v-html` 渲染，可被注入触发 XSS 窃取 token | ✅ 已修复 |
| 🟡 中危 | WebSocket 终端 `CheckOrigin` 恒为 true（跨站 WS 劫持） | ✅ 已修复 |
| 🟡 中危 | CORS `Access-Control-Allow-Origin: *` 任意来源跨域 | ✅ 已修复 |
| 🟡 中危 | 初始管理员密码明文写入 stdout 日志 | ✅ 已修复 |
| 🔵 低危 | 文件管理 `name` 参数未校验（可含 `../`） | ✅ 已修复 |
| 🔵 低危 | AI baseURL 可指向任意地址（SSRF，仅 admin 可配置） | ⚠️ 建议跟进 |
| 🔵 低危 | 应用模版 zip 导入无解压大小限制（zip bomb） | ⚠️ 建议跟进 |
| 🔵 低危 | 官方模版同步信任 manifest 的 DownloadURL（供应链） | ⚠️ 建议跟进 |
| 🔵 低危 | `/metrics` 端点默认无认证开放（信息泄露） | ⚠️ 建议跟进 |
| 🔵 低危 | access/refresh token 未区分类型（access 可当 refresh 用） | ⚠️ 建议跟进 |

---

## 二、严重问题详情与修复

### 1. Agent 命令白名单绕过（严重）✅

**文件**：`internal/api/v1/agent.go`

**问题**：`isAllowed` 用 `strings.HasPrefix(base, allowed)` 做**前缀匹配**，且命令通过 `sh -c` 执行。攻击者可提交 `ps; cat /etc/shadow`，白名单检查只看到 `ps` 前缀即放行，随后 shell 执行 `ps` **和** `cat /etc/shadow`，完全绕过白名单实现任意命令执行。

**修复**：
- 改为命令名**精确匹配**（`base == allowed`）
- 禁止 shell 元字符 `; & | \` $ > < ( ) 换行`，彻底阻断注入链

### 2. JWT Secret 默认弱密钥（严重）✅

**文件**：`internal/config/config.go`、`cmd/agent/main.go`

**问题**：默认配置 `JWT.Secret = "change-me"`。手动部署（直接用 config.yaml 模板）或配置缺失时，攻击者知晓此公开默认值，可直接伪造任意用户（含 admin）的 JWT，完全接管面板。

**修复**：新增 `EnsureSecureSecret`，启动时检测弱密钥（空 / `change-me` / 长度<16），自动生成 32 字节随机密钥，更新内存并**写回配置文件**（用 yaml.Node 保留注释）。安装脚本原本已生成随机密钥，此修复补齐手动部署场景。

---

## 三、高危问题详情与修复

### 3. AI 聊天 v-html XSS（高危）✅

**文件**：`web/index.html`（2 处 AI 聊天气泡）

**问题**：AI 回复用 `v-html="m.text"` 渲染。若 AI 被提示词注入或回显了系统数据（进程名/日志中可能含恶意字符串），恶意 HTML 会被执行，进而读取 `localStorage` 中的 JWT token（token 明文存于 localStorage），导致会话劫持。

**修复**：`v-html` 改为 Vue 插值 `{{ m.text }}`（自动转义），并加 `white-space: pre-wrap` 保留换行格式。全项目已无 `v-html`。

---

## 四、中危问题详情与修复

### 4. WebSocket 终端跨站劫持（中）✅

**文件**：`internal/api/v1/terminal.go`

**问题**：`CheckOrigin` 恒返回 true，任意来源页面可尝试连接终端 WebSocket。

**修复**：校验 `Origin` 的 host 与请求 `Host` 一致，仅放行同源连接。

### 5. CORS 任意来源（中）✅

**文件**：`internal/middleware/cors.go`

**问题**：`Access-Control-Allow-Origin: *`。

**修复**：改为仅当 `Origin` 与请求 `Host` 同源时才回显 CORS 头（并加 `Vary: Origin`）。

### 6. 初始密码明文日志（中）✅

**文件**：`internal/pkg/store/store.go`

**问题**：`seedAdmin` 用 `log.Printf("... password=%s", pwd)` 把初始密码明文写入 stdout（journalctl）。

**修复**：随机生成的密码改写入 `.init_passwd`（权限 0600），日志只提示文件位置，不再输出明文。

---

## 五、低危问题详情

### 7. 文件管理 name 参数校验（低）✅

**文件**：`internal/service/file.go`

**问题**：`MakeDir`/`Rename`/`SaveUploaded` 的 `name`/`newName` 未校验，可含 `../`（当前 root=`/` 影响有限，但属防御性缺陷）。

**修复**：新增 `validateName`，拒绝空值、`.`、`..` 及含 `/`、`\`、`..` 的名称。

---

## 六、建议跟进（未在本轮修复，需评估）

| 问题 | 说明 | 建议 |
|------|------|------|
| AI baseURL SSRF | `ai_base_url` 由面板设置配置，`http.Client` 会跟随重定向 | 仅 admin 可配置，风险低；建议禁用重定向或校验内网地址 |
| zip 导入 zip bomb | `ImportZip` 用 `io.ReadAll` 无大小限制 | 加解压总大小上限（如 100MB） |
| 官方同步供应链 | `SyncOfficial` 信任 manifest 的 `DownloadURL` | 校验域名必须为 `opsmini.com` |
| `/metrics` 无认证 | 默认 `Enabled=true` 且 Token 空 | 建议默认关闭或强制设置 token |
| access/refresh 未区分 | `Generate` 两者 purpose 均为空 | refresh 接口应校验 token 类型 |

---

## 七、已确认安全的方面（正面）

- ✅ 密码使用 **bcrypt** 存储（`GenerateFromPassword` / `CompareHashAndPassword`）
- ✅ 支持 **MFA/TOTP** 双因素认证
- ✅ JWT 校验签名方法（防算法混淆攻击），支持 token 撤销黑名单
- ✅ Agent API 与 Metrics 端点认证使用 **恒定时间比较**（防时序攻击），Agent token 为空时禁用
- ✅ 登录**限流**（10 次/分钟/IP）
- ✅ 文件管理 `resolve` 有 `filepath.Clean` + `filepath.Rel` 路径穿越防护
- ✅ 日志读取有**白名单**（仅允许预定义日志文件）
- ✅ 防火墙命令用**参数数组**（非 shell 拼接）+ 端口/协议白名单 + root 校验 + 面板端口保护
- ✅ 应用模版 zip 导入有 `..` 路径穿越防护
- ✅ 无原生 SQL 拼接（GORM 参数化查询），SQL 注入风险低
- ✅ RBAC 权限点系统（`RequirePerm` + 前端 `hasPerm`）
- ✅ 初始密码**随机生成**（16 位强密码）

---

## 八、后续建议

1. 运行 `govulncheck ./...` 检查依赖已知漏洞（本轮未执行 CVE 扫描）。
2. 对「建议跟进」的 5 项低危问题评估是否纳入下个迭代。
3. 考虑给 `/metrics` 端点默认关闭或强制认证。
4. 生产部署务必通过 `install/install.sh`（`curl -fsSL https://opsmini.com/install.sh | sudo bash`）；JWT 签名密钥与 Agent token 已改为首次启动时自动生成并持久化到数据库，无需（也不应）在 config.yaml 中配置。
