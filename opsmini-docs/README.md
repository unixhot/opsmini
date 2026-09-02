# OpsMini 官方文档

基于 [Material for MkDocs](https://squidfunk.github.io/mkdocs-material/) 的文档站点，发布到 https://www.opsmini.com/docs/。全站 **7 语言**：
简体中文（zh，源语言）、繁體中文（zh-TW）、English（en）、日本語（ja）、한국어（ko）、ไทย（th）、Deutsch（de）。

## 本地构建

```bash
cd opsmini-docs

# 1. 安装依赖
uv venv .venv && source .venv/bin/activate
uv pip install -r requirements.txt

# 2. 本地预览（http://127.0.0.1:8000）
mkdocs serve

# 3. 构建静态产物
mkdocs build
```

## 部署

`mkdocs build` 的产物在 `site/` 目录，需同步到官网（`opsmini-site/`）的 `/docs/` 子目录，与官网一起发布，最终通过 https://www.opsmini.com/docs/ 访问。

```bash
mkdocs build
# 将构建产物同步到官网的 docs/ 子目录
rsync -av --delete site/ ../opsmini-site/docs/
```

> 官网源码在 `opsmini-site/`（发布到 https://www.opsmini.com 根域名），文档站作为其 `/docs/` 子路径提供。

## 目录说明

```
docs/zh/        # 简体中文（默认语言，构建到根路径 /，也是翻译的单一真相源）
docs/zh-TW/     # 繁體中文（URL /zh-TW/）
docs/en/ ja/ ko/ th/ de/   # 其余语言，与 zh/ 目录镜像
docs/assets/    # 全局共享资源（logo/favicon/css/js/images）
overrides/      # Material 主题覆盖（footer 等）
scripts/        # 翻译工具脚本（translate.py + glossary.md 术语表）
```

## 翻译工作流

1. 只维护 `docs/zh/`（中文原创）。
2. 维护 `scripts/glossary.md` 术语表（zh → 6 语言）。
3. 运行 `python scripts/translate.py`，产出 6 语言初稿（机翻）。
4. 关键页（快速开始/安装/API）人工 review，长尾页靠 `fallback_to_default` 回退中文。

## 依赖锁定说明

`mkdocs-material` 9.7 起强制 `mkdocs<2.0`，`mkdocs-static-i18n` 已冻结在 1.3.x。
`requirements.txt` 已锁版本上界，**请勿**随意升级到 mkdocs 2.0。
