# 站点开发与维护备忘

> 本文档由独立开发期（`2026-08-14` 站点搭建、`2026-09-02` 文档重写与合并）的两份工作记录压缩整合而来，保留后续维护真正需要的决策与踩坑经验。正式维护位置为 OpsMini 仓库内的 `website/` 目录。

## 一、站点架构与关键决策

- **技术底座**：Material for MkDocs（mkdocs 1.6.1 + material 9.7.7 + mkdocs-static-i18n 1.3.1）
- **单站点合一**：首页即官网 Landing（Hero/特性卡/一键安装），顶部导航进入「文档」板块
- **7 语言**：`zh`（源语言，构建到根路径 `/`）/ `zh-TW` / `en` / `ja` / `ko` / `th` / `de`，i18n 用 `docs_structure: folder` 模式
- **目录结构**：
  - `docs/zh/` 默认语言（也是翻译的单一真相源）
  - `docs/zh-TW/`、`docs/en/` 等镜像目录
  - `docs/assets/` 全局共享资源（logo/favicon/css）
  - `overrides/` Material 主题覆盖（当前只覆盖 `partials/copyright.html`）
  - `scripts/` 翻译工具（`translate.py` + `glossary.md` 术语表）

## 二、踩坑记录（务必沿用）

1. **mkdocs-static-i18n 1.x 配置格式大改**：旧 0.x 的 `default_language` / `material_alternate` 已废弃，必须用 `languages` 列表 + `reconfigure_material: true`。网上教程大多是 0.x 语法，会直接报错。
2. **繁体中文 locale 必须是 `zh-TW`（连字符）**：Material 语言文件是 `zh-TW.html`，写 `zh_TW`（下划线）会报 `TemplateNotFound`。i18n 的 folder 目录名必须与 locale 一致 → 目录用 `docs/zh-TW/`。
3. **依赖必须锁上界**：material 9.7 起强制 `mkdocs<2`，`requirements.txt` 应写 `mkdocs>=1.6,<2.0`、`mkdocs-material<10`、`mkdocs-static-i18n<2`。**不要**升级到 mkdocs 2.0。
4. **本机 pip 默认源可能卡死**：`uv pip install` 用默认 PyPI 源曾 14 分钟装 0 个包，改用清华镜像 `--index-url https://pypi.tuna.tsinghua.edu.cn/simple` 秒级完成。

## 三、产品事实（供文档维护参考）

- 品牌主色 `#4f6ef7`；公司「北京速云科技有限公司」；协议 Apache 2.0；GitHub `unixhot/opsmini`
- 产品源码位置：`/Users/vicky/WorkBuddy/2026-08-13-09-06-05/`
- 产品功能远超早期 README：含主机安全（基线检查/FIM/威胁检测/防火墙/登录安全）、RBAC 权限点、MCP、Prometheus `/metrics`、双 API（`/api/v1` + `/agent/v1`）

## 四、文档维护历史

### v1.0.25 对齐（2026-09-02）

根据产品最新代码重写文档，核心是配置相关变更：

| 变更 | 说明 |
|------|------|
| JWT 密钥 | `jwt.secret` 不再落配置文件，首次启动自动生成 32 字节随机密钥存库 |
| AI 大模型 | 从 `config.yaml` 的 `ai` 段迁移到面板「设置 → AI 大模型接入」页 |
| Agent 令牌 | `agent.token` 废弃，迁移到面板「设置 → API Token」页 |
| Prometheus 认证 | `metrics.token` 改为 Basic auth（user/password），面板设置优先 |
| 安全入口 | 新增 `server.secret_entry` 路径前缀 |
| 账号恢复 | 新增 CLI 参数 `-reset-mfa` / `-reset-pass` |
| MFA | 新增双因素认证（全局开关 `mfa_enabled` + 每用户 TOTP 绑定） |
| 面板设置 Tab | 新增 AI 接入 / API Token / 监控导出 / 通知 / 应用模版 / 日志保留 |
| Agent API | 新增端点 `/version` `/script/run` `/file/upload` |

文档改动：重写 `configuration/config-file.md`、新增 `configuration/panel-settings.md`、更新 `api/{agent-api,browser-api,prometheus}.md`、`features/{ai-assistant,users-rbac}.md`、`getting-started/first-steps.md`、`changelog/index.md`；同步英文版并 opencc 重新生成繁体。

## 五、翻译工作流

1. 只维护 `docs/zh/`（中文原创）。
2. 维护 `scripts/glossary.md` 术语表（zh → 6 语言）。
3. 运行 `python scripts/translate.py`，产出其余语言初稿。
4. 关键页（快速开始/安装/API）人工 review，长尾页靠 `fallback_to_default` 回退中文。

**当前语言完成度**：zh 40 篇（全量）/ zh-TW 40 篇（opencc 转换）/ en 21 篇（核心章节）/ ja、ko、th、de 靠 fallback 回退中文，待机翻补齐。
