#!/usr/bin/env python3
"""OpsMini 文档机翻脚本。

将 docs/zh/ 下的中文源文档翻译到其余 6 种语言目录：
  - zh-TW：本地简繁转换（opencc），无需 API
  - en / ja / ko / th / de：调用 OpenAI 兼容 LLM API 机翻

用法：
  python scripts/translate.py --lang en          # 翻译单个语言
  python scripts/translate.py --all              # 翻译全部 6 语言
  python scripts/translate.py --lang en --dry-run

环境变量（LLM 翻译需要）：
  OPSMINI_TRANSLATE_BASE_URL   如 https://api.deepseek.com/v1
  OPSMINI_TRANSLATE_API_KEY    你的 API Key
  OPSMINI_TRANSLATE_MODEL      如 deepseek-chat（默认 deepseek-chat）

zh-TW 依赖：pip install opencc-python-reimplemented
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SRC_DIR = ROOT / "docs" / "zh"
GLOSSARY = ROOT / "scripts" / "glossary.md"
STATE_FILE = ROOT / "scripts" / ".translate_state.json"

LANGS = {
    "zh-TW": "繁體中文",
    "en": "English",
    "ja": "日本語",
    "ko": "한국어",
    "th": "ไทย",
    "de": "Deutsch",
}

# 需要保护、不翻译的 markdown 结构
RE_FENCE = re.compile(r"```.*?```", re.DOTALL)
RE_INLINE_CODE = re.compile(r"`[^`]+`")
RE_FRONT_MATTER = re.compile(r"^---\n.*?\n---\n", re.DOTALL)
RE_LINK_URL = re.compile(r"\]\(([^)]*)\)")
RE_HTML_TAG = re.compile(r"<[^>]+>")
RE_TABLE_SEP = re.compile(r"^\s*\|[\s\-:|]+\|\s*$", re.MULTILINE)


def load_glossary() -> dict[str, dict[str, str]]:
    """解析 glossary.md，返回 {zh_term: {lang: translated_term}}"""
    glossary: dict[str, dict[str, str]] = {}
    if not GLOSSARY.exists():
        return glossary
    text = GLOSSARY.read_text(encoding="utf-8")
    header: list[str] = []
    for line in text.splitlines():
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        if line.lstrip().startswith("|") and "zh" in line and "en" in line:
            # 表头行，解析列顺序
            cells = [c.strip() for c in line.strip().strip("|").split("|")]
            header = cells
            continue
        if not header or not line.lstrip().startswith("|"):
            continue
        cells = [c.strip() for c in line.strip().strip("|").split("|")]
        if len(cells) < len(header):
            continue
        row = dict(zip(header, cells))
        zh = row.get("zh", "")
        if not zh:
            continue
        glossary[zh] = {}
        for lang in LANGS:
            if lang in row and row[lang]:
                glossary[zh][lang] = row[lang]
    return glossary


def protect(text: str) -> tuple[str, list[str]]:
    """把不可翻译的片段替换为占位符，返回 (新文本, 占位符列表)"""
    tokens: list[str] = []

    def stash(match: re.Match) -> str:
        tokens.append(match.group(0))
        return f"\x00{len(tokens) - 1}\x00"

    text = RE_FRONT_MATTER.sub(stash, text)
    text = RE_FENCE.sub(stash, text)
    text = RE_INLINE_CODE.sub(stash, text)
    text = RE_HTML_TAG.sub(stash, text)
    text = RE_TABLE_SEP.sub(stash, text)

    # 链接 URL 只保护 URL 部分
    def stash_url(match: re.Match) -> str:
        tokens.append(match.group(1))
        return f"]({chr(1)}{len(tokens) - 1}{chr(1)})"

    text = RE_LINK_URL.sub(stash_url, text)
    return text, tokens


def restore(text: str, tokens: list[str]) -> str:
    """把占位符还原为原始内容"""
    def un_url(match: re.Match) -> str:
        return tokens[int(match.group(1))]

    text = re.sub(rf"\]\({chr(1)}(\d+){chr(1)}\)", un_url, text)
    text = re.sub(r"\x00(\d+)\x00", lambda m: tokens[int(m.group(1))], text)
    return text


def apply_glossary(text: str, glossary: dict[str, dict[str, str]], lang: str) -> str:
    """按术语表替换目标语言术语（仅在还原后的 prose 区生效）"""
    for zh, mapping in glossary.items():
        if lang in mapping and zh in text:
            text = text.replace(zh, mapping[lang])
    return text


def translate_zh_tw(text: str, glossary: dict[str, dict[str, str]]) -> str:
    try:
        from opencc import OpenCC
    except ImportError:
        sys.exit("zh-TW 翻译需要 opencc：pip install opencc-python-reimplemented")
    cc = OpenCC("s2twp")
    return apply_glossary(cc.convert(text), glossary, "zh-TW")


def translate_llm(text: str, lang: str, glossary: dict[str, dict[str, str]]) -> str:
    import urllib.request

    base_url = os.environ.get("OPSMINI_TRANSLATE_BASE_URL", "").rstrip("/")
    api_key = os.environ.get("OPSMINI_TRANSLATE_API_KEY", "")
    model = os.environ.get("OPSMINI_TRANSLATE_MODEL", "deepseek-chat")
    if not base_url or not api_key:
        sys.exit("缺少 OPSMINI_TRANSLATE_BASE_URL / OPSMINI_TRANSLATE_API_KEY 环境变量")

    lang_name = LANGS[lang]
    terms = "\n".join(
        f"- {zh} → {m.get(lang, '')}" for zh, m in glossary.items() if lang in m
    ) or "（无）"

    system = (
        f"你是专业的技术文档翻译。将中文翻译为{lang_name}。\n"
        "要求：\n"
        "1. 只翻译自然语言，保留所有代码、命令、路径、版本号、URL 原样\n"
        "2. 保留 Markdown 结构、链接、表格、HTML 标签\n"
        f"3. 术语必须严格遵循以下对照表（不要自行翻译术语）：\n{terms}\n"
        "4. 技术名词（OpsMini/REST API/JWT/RBAC/Docker/Nginx 等）保持英文原文\n"
        "5. 直接输出译文，不要任何解释或前后缀"
    )

    payload = json.dumps({
        "model": model,
        "messages": [
            {"role": "system", "content": system},
            {"role": "user", "content": text},
        ],
        "temperature": 0.2,
    }).encode("utf-8")

    req = urllib.request.Request(
        f"{base_url}/chat/completions",
        data=payload,
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {api_key}",
        },
        method="POST",
    )
    with urllib.request.urlopen(req, timeout=120) as resp:
        data = json.loads(resp.read().decode("utf-8"))
    return data["choices"][0]["message"]["content"]


def translate_file(src: Path, dst: Path, lang: str, glossary) -> None:
    raw = src.read_text(encoding="utf-8")
    protected, tokens = protect(raw)

    if lang == "zh-TW":
        out = translate_zh_tw(protected, glossary)
    else:
        out = translate_llm(protected, lang, glossary)

    out = restore(out, tokens)
    dst.parent.mkdir(parents=True, exist_ok=True)
    dst.write_text(out, encoding="utf-8")
    print(f"  ✓ {dst.relative_to(ROOT)}")


def load_state() -> dict[str, list[str]]:
    if STATE_FILE.exists():
        return json.loads(STATE_FILE.read_text(encoding="utf-8"))
    return {}


def save_state(state: dict[str, list[str]]) -> None:
    STATE_FILE.write_text(json.dumps(state, ensure_ascii=False, indent=2), encoding="utf-8")


def main() -> None:
    parser = argparse.ArgumentParser(description="OpsMini 文档机翻")
    parser.add_argument("--lang", choices=list(LANGS), help="目标语言")
    parser.add_argument("--all", action="store_true", help="翻译全部 6 语言")
    parser.add_argument("--dry-run", action="store_true", help="只列出待翻译文件")
    parser.add_argument("--force", action="store_true", help="覆盖已翻译文件")
    args = parser.parse_args()

    if not args.lang and not args.all:
        parser.error("需要 --lang 或 --all")

    langs = list(LANGS) if args.all else [args.lang]
    glossary = load_glossary()
    state = load_state()
    md_files = sorted(SRC_DIR.rglob("*.md"))

    for lang in langs:
        done = set(state.get(lang, []))
        print(f"\n=== {lang} ({LANGS[lang]}) ===")
        for src in md_files:
            rel = src.relative_to(SRC_DIR)
            key = str(rel)
            if key in done and not args.force:
                continue
            dst = ROOT / "docs" / lang / rel
            if args.dry_run:
                print(f"  - {rel}")
                continue
            print(f"  · {rel}")
            try:
                translate_file(src, dst, lang, glossary)
                done.add(key)
                state[lang] = sorted(done)
                save_state(state)
            except Exception as e:  # noqa: BLE001
                print(f"  ✗ 失败: {e}")
                continue
        print(f"  完成，共 {len([f for f in md_files if str(f.relative_to(SRC_DIR)) in done])}/{len(md_files)}")


if __name__ == "__main__":
    main()
