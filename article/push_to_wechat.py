#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
把 wechat-article.html 直接推送到微信公众号「草稿箱」。

前置条件（缺一不可）：
  1. 公众号（订阅号/服务号均可调草稿箱接口；部分高级接口才需要认证）
  2. 拿 AppID / AppSecret：微信已把「设置与开发 → 开发接口管理」整体迁移到
     微信开发者平台 https://developers.weixin.qq.com/console/product/mp/<AppID>?tab1=basicInfo
     旧后台 mp.weixin.qq.com 的「基本配置」页已停用，只能看到迁移通知。
     AppSecret 不可查看，只有「重置」按钮：管理员扫码 + 短信验证后明文只显示一次。
  3. 同一页面的「API IP 白名单」里加入本机公网出口 IP（否则接口报 40164）

用法：
  export WX_APPID=wx************
  export WX_SECRET=****************
  python3 push_to_wechat.py --dry-run        # 先空跑，看会推送什么
  python3 push_to_wechat.py                  # 真推，进草稿箱等你点发布

说明：
  - 正文里的图片会被逐张上传到微信（media/uploadimg）换取 mmbiz.qpic.cn 直链，
    外链图片在公众号正文里是不显示的，必须走这一步。
  - 封面图取 assets/diagram-architecture.png，作为永久图片素材上传换取 thumb_media_id。
  - 只创建草稿，不群发。群发需要额外权限（认证服务号 + freepublish 接口），
    建议先在后台预览确认排版，再手动发布。
"""

import argparse
import json
import mimetypes
import os
import re
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid
from pathlib import Path

API = "https://api.weixin.qq.com"
HERE = Path(__file__).resolve().parent

TITLE = "OpsMini：把 AI 装进服务器，让它自己运维自己"
DIGEST = (
    "监控 Agent 只上报，面板只等人点。OpsMini 是第三种：装上去就常驻，"
    "自己发现、自己判断、自己动手，高危动作必等你点头。单二进制，开源。"
)
AUTHOR = "OpsMini"
SOURCE_URL = "https://github.com/unixhot/opsmini/"


# ---------------------------------------------------------------- HTTP 工具

def _request(url, data=None, headers=None, method=None):
    req = urllib.request.Request(url, data=data, headers=headers or {}, method=method)
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            raw = resp.read()
    except urllib.error.HTTPError as e:
        raw = e.read()
    try:
        return json.loads(raw.decode("utf-8"))
    except Exception:
        raise SystemExit(f"接口返回非 JSON：{raw[:300]!r}")


def _check(payload, what):
    if "errcode" in payload and payload["errcode"] != 0:
        code = payload["errcode"]
        msg = payload.get("errmsg", "")
        hint = ""
        if code == 40164:
            hint = (
                "\n→ IP 不在白名单：把本机公网 IP 加到 "
                "https://developers.weixin.qq.com/console/product/mp/<AppID>?tab1=basicInfo"
                " 的「API IP 白名单」"
            )
        elif code in (40001, 40002, 40125):
            hint = "\n→ AppSecret 不对，重新复制一次（注意别带空格）"
        elif code == 48001:
            hint = (
                "\n→ 账号没有该接口权限。实测判断：公众号未通过微信认证"
                "（getaccountbasicinfo 里 qualification_verify=false）时，"
                "素材（material/*）与草稿箱（draft/*）接口全部不可用。\n"
                "   解法只有两个：① 做微信认证（300 元/年）后本脚本可原样使用；"
                "② 手动粘贴（见 wechat-article.html 头部的配图清单）。"
            )
        elif code == 45009:
            hint = "\n→ 接口调用超限，等一会儿再试"
        raise SystemExit(f"{what} 失败：[{code}] {msg}{hint}")
    return payload


def post_json(url, obj):
    body = json.dumps(obj, ensure_ascii=False).encode("utf-8")
    return _request(url, data=body, headers={"Content-Type": "application/json"})


def post_multipart(url, fields, file_field, file_path):
    """极简 multipart/form-data，仅供微信素材上传使用。"""
    boundary = "----OpsMini" + uuid.uuid4().hex
    path = Path(file_path)
    ctype = mimetypes.guess_type(path.name)[0] or "application/octet-stream"
    parts = []
    for k, v in (fields or {}).items():
        parts.append(
            f"--{boundary}\r\nContent-Disposition: form-data; name=\"{k}\"\r\n\r\n{v}\r\n".encode()
        )
    parts.append(
        f"--{boundary}\r\nContent-Disposition: form-data; name=\"{file_field}\"; "
        f"filename=\"{path.name}\"\r\nContent-Type: {ctype}\r\n\r\n".encode()
    )
    parts.append(path.read_bytes())
    parts.append(f"\r\n--{boundary}--\r\n".encode())
    body = b"".join(parts)
    headers = {"Content-Type": f"multipart/form-data; boundary={boundary}"}
    return _request(url, data=body, headers=headers)


# ---------------------------------------------------------------- 微信接口

def get_access_token(appid, secret):
    payload = post_json(
        f"{API}/cgi-bin/stable_token",
        {"grant_type": "client_credential", "appid": appid, "secret": secret, "force_refresh": False},
    )
    return _check(payload, "获取 access_token")["access_token"]


def upload_content_image(token, image_path):
    """正文图片 → 返回 mmbiz.qpic.cn 直链（不占永久素材额度）。"""
    url = f"{API}/cgi-bin/media/uploadimg?access_token={urllib.parse.quote(token)}"
    payload = post_multipart(url, {}, "media", image_path)
    return _check(payload, f"上传正文图片 {Path(image_path).name}")["url"]


def upload_cover(token, image_path):
    """封面图 → 永久图片素材，拿 media_id 给 thumb_media_id 用。"""
    url = (
        f"{API}/cgi-bin/material/add_material"
        f"?access_token={urllib.parse.quote(token)}&type=image"
    )
    payload = post_multipart(url, {}, "media", image_path)
    return _check(payload, "上传封面图")["media_id"]


# ---------------------------------------------------------------- 正文处理

def extract_fragment(html):
    """只保留 <body> 里那段内容 div，去掉 html/head/注释等外壳。"""
    html = re.sub(r"<!--.*?-->", "", html, flags=re.S)
    body = re.search(r"<body[^>]*>(.*)</body>", html, flags=re.S)
    inner = body.group(1) if body else html
    # 公众号正文不支持 class / id 选择器，内联样式已足够，清除残留的 class
    inner = re.sub(r'\sclass="[^"]*"', "", inner)
    return inner.strip()


def rewrite_images(html, base_dir, token, dry_run=False):
    """把 <img src="assets/x.png"> 换成微信直链。"""
    seen = {}

    def repl(m):
        tag, src = m.group(0), m.group(2)
        if src.startswith(("http://", "https://", "data:")):
            return tag
        path = (base_dir / src).resolve()
        if not path.exists():
            print(f"  ! 图片不存在，跳过：{src}")
            return tag
        if src not in seen:
            if dry_run:
                seen[src] = f"<DRY-RUN-URL:{path.name}>"
                print(f"  · 会上传 {path.name} ({path.stat().st_size // 1024} KB)")
            else:
                seen[src] = upload_content_image(token, path)
                print(f"  ✓ {path.name} → {seen[src][:60]}…")
        return tag.replace(src, seen[src], 1)

    return re.sub(r'<img([^>]*?)src="([^"]+)"', repl, html)


# ---------------------------------------------------------------- 主流程

def main():
    ap = argparse.ArgumentParser(description="推送 OpsMini 公众号文章到草稿箱")
    ap.add_argument("--file", default=str(HERE / "wechat-article.html"))
    ap.add_argument("--cover", default=str(HERE / "assets" / "diagram-architecture.png"))
    ap.add_argument("--dry-run", action="store_true", help="只本地处理，不调微信写接口")
    args = ap.parse_args()

    appid = os.environ.get("WX_APPID", "").strip()
    secret = os.environ.get("WX_SECRET", "").strip()
    if not args.dry_run and not (appid and secret):
        sys.exit("缺少环境变量 WX_APPID / WX_SECRET（可先用 --dry-run 空跑）")

    src_file = Path(args.file)
    html = src_file.read_text(encoding="utf-8")
    content = extract_fragment(html)
    print(f"正文长度：{len(content)} 字符（公众号上限约 20000）")
    if len(content) > 20000:
        print("  ! 超过上限，建议拆分或精简")

    token = "" if args.dry_run else get_access_token(appid, secret)
    print("处理正文图片：")
    content = rewrite_images(content, src_file.parent, token, args.dry_run)

    thumb_media_id = "DRY-RUN-THUMB"
    if not args.dry_run:
        print("上传封面图：")
        thumb_media_id = upload_cover(token, args.cover)
        print(f"  ✓ thumb_media_id = {thumb_media_id}")

    article = {
        "title": TITLE,
        "author": AUTHOR,
        "digest": DIGEST,
        "content": content,
        "content_source_url": SOURCE_URL,
        "thumb_media_id": thumb_media_id,
        "need_open_comment": 0,
        "only_fans_can_comment": 0,
    }

    if args.dry_run:
        out = HERE / "dry-run-content.html"
        out.write_text(content, encoding="utf-8")
        print(f"\n空跑完成，处理后的正文已写入 {out}")
        print("确认无误后设置 WX_APPID / WX_SECRET 再跑一次即可进草稿箱。")
        return

    print("创建草稿：")
    payload = post_json(
        f"{API}/cgi-bin/draft/add?access_token={urllib.parse.quote(token)}",
        {"articles": [article]},
    )
    media_id = _check(payload, "创建草稿")["media_id"]
    print(f"  ✓ 草稿已创建，media_id = {media_id}")
    print("\n下一步：登录 mp.weixin.qq.com → 内容管理 → 草稿箱，预览排版后点发布。")


if __name__ == "__main__":
    main()
