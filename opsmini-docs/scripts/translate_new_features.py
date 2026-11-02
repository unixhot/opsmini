#!/usr/bin/env python3
"""把 v1.4 新增的功能页翻成 4 种语言（ja / ko / th / de）。

复用 translate_hostsec.py 的分块与校验逻辑（整篇一次调用会被输出上限截断），
只处理指定文件列表，不碰已有的人工译文。

用法:
    python scripts/translate_new_features.py --lang ja
    python scripts/translate_new_features.py --all
"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))

import translate_hostsec as H  # noqa: E402

ROOT = Path(__file__).resolve().parent.parent

FILES = [
    "features/autonomous-agent.md",
    "features/change-awareness.md",
    "features/log-pipeline.md",
    "features/plugin-system.md",
    "features/backup-restore.md",
]
LANGS = ["ja", "ko", "th", "de"]


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--lang", choices=LANGS)
    ap.add_argument("--all", action="store_true")
    ap.add_argument("--file", help="只翻一个文件（相对 docs/zh）")
    args = ap.parse_args()
    if not args.lang and not args.all:
        ap.error("需要 --lang 或 --all")

    glossary = H.T.load_glossary()
    files = [args.file] if args.file else FILES
    for lang in ([args.lang] if args.lang else LANGS):
        print(f"=== {lang} ===")
        for rel in files:
            src = ROOT / "docs" / "zh" / rel
            dst = ROOT / "docs" / lang / rel
            text = src.read_text(encoding="utf-8")
            try:
                out = H.translate_doc(text, lang, glossary)
            except Exception as e:  # noqa: BLE001
                print(f"  ✗ {rel}: {e}")
                continue
            base, got = H.stats(text), H.stats(out)
            if base != got:
                print(f"  ! {rel} 结构不一致 base={base} got={got}（仍写入，需人工复核）")
            dst.parent.mkdir(parents=True, exist_ok=True)
            dst.write_text(out, encoding="utf-8")
            print(f"  ✓ {dst.relative_to(ROOT)}")
    print("done")


if __name__ == "__main__":
    main()
