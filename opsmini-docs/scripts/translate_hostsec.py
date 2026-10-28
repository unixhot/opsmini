#!/usr/bin/env python3
"""把主机安全设计文档翻成 6 种语言。

与 scripts/translate.py 的区别（踩过的坑）：
  1. 按 ## / ### 分块调用 LLM —— 整篇一次调用会被输出上限截断
  2. 占位符用 [[PH0]] 这类 ASCII 形态。translate.py 用的 \\x00 控制字符会被模型
     吃掉或改写成数字，导致表格分隔行、代码围栏丢失
  3. 代码围栏不保护（模型会被明确要求原样保留，保护反而更容易丢），
     只保护行内代码与链接 URL
  4. 输出后做结构修复与校验：表格缺分隔行就补、列数不齐就对齐；
     标题/围栏/表格行数与原文不一致直接报错、不落盘

用法：
    python scripts/translate_hostsec.py --lang en
    python scripts/translate_hostsec.py --all
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import translate as T  # noqa: E402

ROOT = T.ROOT
SRC = ROOT / "docs" / "zh" / "architecture" / "host-security.md"
SITE_FMT = ROOT / "docs" / "{lang}" / "architecture" / "host-security.md"
REPO_FMT = ROOT.parent / "docs" / "host-security-design.{suffix}.md"

SUFFIX = {"zh-TW": "zh-TW", "en": "en", "ja": "ja", "ko": "ko", "th": "th", "de": "de"}
LOCAL_TITLE = {
    "zh-TW": "主機安全設計",
    "en": "Host Security Design",
    "ja": "ホストセキュリティ設計",
    "ko": "호스트 보안 설계",
    "th": "การออกแบบความปลอดภัยโฮสต์",
    "de": "Host-Sicherheitsdesign",
}

MAX_CHARS = 3500
RE_PH = re.compile(r"\[\[PH(\d+)\]\]")
RE_FENCE_LINE = re.compile(r"(?m)^```")
RE_TABLE_SEP = re.compile(r"^\|(\s*:?-{2,}:?\s*\|)+\s*$")


# ---------- 分块 ----------

def split_chunks(md: str) -> list[str]:
    """按 ## 切块；过大的块再按 ### 切；保证代码围栏不跨块。"""
    chunks: list[str] = []
    buf = ""

    def flush():
        nonlocal buf
        if buf.strip():
            chunks.append(buf)
        buf = ""

    for block in re.split(r"(?m)^(?=## )", md):
        if not block.strip():
            continue
        if len(block) <= MAX_CHARS:
            flush()
            chunks.append(block)
            continue
        for sub in re.split(r"(?m)^(?=### )", block):
            if len(buf) + len(sub) > MAX_CHARS and buf:
                flush()
            buf += sub
    flush()

    # 围栏数为奇数的块说明代码块被切开，并到下一块
    merged: list[str] = []
    pending = ""
    for c in chunks:
        c = pending + c
        pending = ""
        if c.count("```") % 2 == 1:
            pending = c
            continue
        merged.append(c)
    if pending:
        merged.append(pending)
    return merged


# ---------- 占位保护（只用 ASCII 形态） ----------

def protect(text: str) -> tuple[str, list[str]]:
    tokens: list[str] = []

    def stash(match: re.Match) -> str:
        tokens.append(match.group(0))
        return f"[[PH{len(tokens) - 1}]]"

    text = re.sub(r"`[^`\n]+`", stash, text)          # 行内代码
    text = re.sub(r"\]\(([^)\s]+)\)", lambda m: "]" + stash(m), text)  # 链接 URL
    return text, tokens


def restore(text: str, tokens: list[str]) -> str:
    def sub(m: re.Match) -> str:
        i = int(m.group(1))
        return tokens[i] if i < len(tokens) else m.group(0)

    return RE_PH.sub(sub, text)


# ---------- 表格结构修复 ----------

def split_row(line: str) -> list[str]:
    s = line.strip()
    if s.startswith("|"):
        s = s[1:]
    if s.endswith("|"):
        s = s[:-1]
    return [c.strip() for c in s.split("|")]


def fix_tables(md: str) -> str:
    """保证每个表格块都有分隔行，且每行列数与表头一致。"""
    lines = md.split("\n")
    out: list[str] = []
    i = 0
    while i < len(lines):
        line = lines[i]
        if not line.lstrip().startswith("|"):
            out.append(line)
            i += 1
            continue

        # 收集一个表格块
        block: list[str] = []
        while i < len(lines) and lines[i].lstrip().startswith("|"):
            block.append(lines[i])
            i += 1

        header = split_row(block[0])
        ncols = len(header)
        body = block[1:]
        # 去掉模型生成的错误分隔行
        body = [r for r in body if not RE_TABLE_SEP.match(r.strip())]
        body = [r for r in body if r.strip() not in ("|", "||")]

        out.append("| " + " | ".join(header) + " |")
        out.append("|" + "|".join([" --- "] * ncols) + "|")
        for r in body:
            cells = split_row(r)
            if len(cells) < ncols:
                cells += [""] * (ncols - len(cells))
            elif len(cells) > ncols:
                # 多出来的列并进最后一格，避免丢内容
                cells = cells[:ncols - 1] + [" ".join(cells[ncols - 1:])]
            out.append("| " + " | ".join(cells) + " |")
    return "\n".join(out)


# ---------- LLM ----------

def build_system(lang: str, glossary: dict) -> str:
    terms = "\n".join(
        f"- {zh} → {m.get(lang, '')}" for zh, m in glossary.items() if lang in m
    ) or "（无）"
    return (
        f"你是专业的技术文档翻译。把中文 Markdown 片段翻译成{T.LANGS[lang]}。\n"
        "硬性要求：\n"
        "1. 只翻译自然语言。代码块（``` 包裹的内容）、行内代码、命令、路径、字段名、"
        "版本号、URL 一律原样复制，一个字符都不要改\n"
        "2. 完整保留 Markdown 结构：每个 ## / ### 标题都要在，表格每行每列都要在，"
        "代码围栏 ``` 必须成对出现\n"
        "3. [[PH0]] 这类占位符是受保护片段，必须原样保留、不要改写、不要删除\n"
        "4. 术语遵循对照表，不要自创译法：\n" + terms + "\n"
        "5. OpsMini / REST API / JWT / RBAC / Docker / SQLite / systemd 等技术名词保持英文\n"
        "6. 只输出译文本身：不要解释、不要用 ``` 包裹整段输出"
    )


def call_llm(system: str, user: str, retries: int = 3) -> str:
    base = os.environ.get("OPSMINI_TRANSLATE_BASE_URL", "https://api.deepseek.com/v1").rstrip("/")
    key = os.environ.get("OPSMINI_TRANSLATE_API_KEY", "")
    model = os.environ.get("OPSMINI_TRANSLATE_MODEL", "deepseek-chat")
    if not key:
        sys.exit("缺少 OPSMINI_TRANSLATE_API_KEY")
    payload = json.dumps({
        "model": model,
        "messages": [{"role": "system", "content": system},
                     {"role": "user", "content": user}],
        "temperature": 0.1,
        "max_tokens": 8000,
    }).encode()
    for attempt in range(retries):
        try:
            req = urllib.request.Request(
                f"{base}/chat/completions", data=payload,
                headers={"Content-Type": "application/json",
                         "Authorization": f"Bearer {key}"},
                method="POST")
            with urllib.request.urlopen(req, timeout=240) as resp:
                data = json.loads(resp.read().decode())
            return data["choices"][0]["message"]["content"]
        except (urllib.error.URLError, TimeoutError, KeyError, json.JSONDecodeError) as e:
            if attempt == retries - 1:
                raise
            print(f"    retry {attempt + 1}: {e}")
            time.sleep(2 * (attempt + 1))
    raise RuntimeError("unreachable")


def strip_outer_fence(text: str) -> str:
    t = text.strip()
    if t.startswith("```") and t.endswith("```") and t.count("```") == 2:
        t = re.sub(r"^```[a-zA-Z]*\n", "", t)
        t = re.sub(r"\n```$", "", t)
    return t


# ---------- 主流程 ----------

def stats(md: str) -> dict:
    return {
        "h2": len(re.findall(r"(?m)^## ", md)),
        "h3": len(re.findall(r"(?m)^### ", md)),
        "table_rows": len([l for l in md.splitlines() if l.lstrip().startswith("|")]),
        "fences": md.count("```"),
    }


def translate_doc(text: str, lang: str, glossary: dict) -> str:
    chunks = split_chunks(text)
    system = build_system(lang, glossary)
    out: list[str] = []
    for i, chunk in enumerate(chunks, 1):
        protected, tokens = protect(chunk)
        if lang == "zh-TW":
            body = T.translate_zh_tw(protected, glossary)
        else:
            print(f"    chunk {i}/{len(chunks)} ({len(chunk)} chars)")
            body = strip_outer_fence(call_llm(system, protected))
        body = restore(body, tokens)
        # 保留原块的尾部空行，避免 "---" 与下一个标题粘在一起
        trail = len(chunk) - len(chunk.rstrip("\n"))
        out.append(body.rstrip("\n") + "\n" * max(trail, 1))
    return "".join(out)


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--lang", choices=list(T.LANGS))
    ap.add_argument("--all", action="store_true")
    args = ap.parse_args()
    if not args.lang and not args.all:
        ap.error("需要 --lang 或 --all")

    src_text = SRC.read_text(encoding="utf-8")
    base = stats(src_text)
    print(f"source: {SRC.relative_to(ROOT)}  {base}")

    glossary = T.load_glossary()
    for lang in ([args.lang] if args.lang else list(T.LANGS)):
        print(f"\n=== {lang} ===")
        body = translate_doc(src_text, lang, glossary)
        body = fix_tables(body)
        got = stats(body)
        ok = (got["h2"] == base["h2"] and got["h3"] == base["h3"]
              and got["fences"] == base["fences"] and got["table_rows"] >= base["table_rows"])
        print(f"  {'structure ok' if ok else 'STRUCTURE MISMATCH'}: {got} (source {base})")
        if not ok:
            print("  → 未写入，先修 prompt 或分块")
            continue

        site = Path(str(SITE_FMT).format(lang=lang))
        site.parent.mkdir(parents=True, exist_ok=True)
        site.write_text(body, encoding="utf-8")
        print(f"  ✓ {site.relative_to(ROOT)}")

        links = " · ".join(
            f"[{n}]({f})" for n, f in [
                ("English", "host-security-design.en.md"),
                ("简体中文", "host-security-design.md"),
                ("繁體中文", "host-security-design.zh-TW.md"),
                ("日本語", "host-security-design.ja.md"),
                ("한국어", "host-security-design.ko.md"),
                ("ไทย", "host-security-design.th.md"),
                ("Deutsch", "host-security-design.de.md"),
            ])
        body_wo_h1 = re.sub(r"(?m)^#\s+.*?\n", "", body, count=1).lstrip("\n")
        header = ('<div align="center">\n\n'
                  f"# OpsMini Host Security Design\n\n"
                  f"**{LOCAL_TITLE[lang]}**\n\n{links}\n\n</div>\n\n---\n\n")
        rep = Path(str(REPO_FMT).format(suffix=SUFFIX[lang]))
        rep.write_text(header + body_wo_h1, encoding="utf-8")
        print(f"  ✓ {rep.relative_to(ROOT.parent)}")


if __name__ == "__main__":
    main()
