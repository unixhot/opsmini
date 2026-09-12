#!/usr/bin/env bash
#
# OpsMini 安装脚本
#
# 功能：
#   - 默认安装到 /data/opsmini
#   - 默认端口 8888
#   - 默认从阿里云 OSS（opsmini.oss-cn-beijing.aliyuncs.com）下载对应架构的二进制
#   - 首次启动随机生成管理员密码，写入 <安装目录>/.init_passwd 并展示
#   - 注册 systemd 服务 opsmini.service（开机自启）
#
# 用法：
#   curl -fsSL https://opsmini.com/install.sh | sudo bash   # 一键安装（推荐）
#   sudo ./install.sh -b ./opsmini                          # 指定本地二进制
#   sudo ./install.sh -u https://.../opsmini.tar.gz         # 自定义下载地址
#   sudo ./install.sh -d /opt/opsmini -p 9999               # 自定义目录与端口
#
set -euo pipefail

# ===== 默认参数 =====
INSTALL_DIR="/data/opsmini"
PORT=8888
HOST="0.0.0.0"
BINARY=""
BINARY_URL=""
VERSION="v1.0.0"                                                    # 默认下载的版本号（发布新版本时更新）
DEFAULT_URL_BASE="https://opsmini.oss-cn-beijing.aliyuncs.com"     # 阿里云 OSS 下载地址
NO_SYSTEMD=0

# ===== 颜色 =====
C_GREEN='\033[0;32m'
C_YELLOW='\033[1;33m'
C_RED='\033[0;31m'
C_BOLD='\033[1m'
C_RESET='\033[0m'

info()  { echo -e "${C_GREEN}[INFO]${C_RESET} $*"; }
warn()  { echo -e "${C_YELLOW}[WARN]${C_RESET} $*"; }
error() { echo -e "${C_RED}[ERROR]${C_RESET} $*"; }

usage() {
  cat <<EOF
OpsMini 安装脚本

用法: $0 [选项]

选项:
  -d, --dir <path>     安装目录（默认 ${INSTALL_DIR}）
  -p, --port <port>    面板监听端口（默认 ${PORT}）
  -b, --binary <path>  opsmini 二进制路径
  -u, --url <url>      从 URL 下载二进制（支持 .tar.gz 或裸二进制）
  -n, --no-systemd     不注册 systemd 服务（仅解压 + 初始化）
  -h, --help           显示本帮助

示例:
  curl -fsSL https://opsmini.com/install.sh | sudo bash
  $0 -b ./dist/opsmini-dev-linux-amd64
  $0 -u https://opsmini.oss-cn-beijing.aliyuncs.com/opsmini-v1.0.0-linux-amd64
EOF
}

# ===== 解析参数 =====
while [ $# -gt 0 ]; do
  case "$1" in
    -d|--dir)    INSTALL_DIR="$2"; shift 2 ;;
    -p|--port)   PORT="$2"; shift 2 ;;
    -b|--binary) BINARY="$2"; shift 2 ;;
    -u|--url)    BINARY_URL="$2"; shift 2 ;;
    -n|--no-systemd) NO_SYSTEMD=1; shift ;;
    -h|--help)   usage; exit 0 ;;
    *) echo "未知参数: $1"; usage; exit 1 ;;
  esac
done

# ===== 权限检查 =====
if [ "$(id -u)" -ne 0 ]; then
  error "请使用 root 执行（需要写 ${INSTALL_DIR} 和注册 systemd 服务）"
  exit 1
fi

# ===== 架构检测 =====
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  GOARCH="amd64" ;;
  aarch64|arm64) GOARCH="arm64" ;;
  *) error "不支持的架构: ${ARCH}（仅支持 x86_64 / aarch64）"; exit 1 ;;
esac
info "检测到架构: ${ARCH} (${GOARCH})"

# ===== 定位二进制 =====
resolve_binary() {
  local candidate=""

  # 1. 显式指定
  if [ -n "$BINARY" ]; then
    candidate="$BINARY"
  fi

  # 2. 脚本同目录下的常见产物
  if [ -z "$candidate" ]; then
    local script_dir="$(cd "$(dirname "$0")" && pwd)"
    for f in \
      "$script_dir/opsmini" \
      "$script_dir/opsmini-linux-${GOARCH}" \
      "$script_dir/dist/opsmini-linux-${GOARCH}" \
      "$script_dir/dist/opsmini-dev-linux-${GOARCH}" \
      "$script_dir/../dist/opsmini-linux-${GOARCH}" \
      "$script_dir/../dist/opsmini-dev-linux-${GOARCH}" \
      "$script_dir/../opsmini"; do
      if [ -f "$f" ]; then candidate="$f"; break; fi
    done
  fi

  if [ -z "$candidate" ]; then
    error "未找到 opsmini 二进制，请用 -b 指定路径或 -u 指定下载地址"
    exit 1
  fi
  echo "$candidate"
}

# ===== 随机生成工具 =====
rand_str() {
  # $1 长度；$2 是否去易混淆字符
  local n="$1" exclude="$2" s
  s="$(LC_ALL=C tr -dc 'a-zA-Z0-9' </dev/urandom | head -c "$((n*3))")"
  if [ "$exclude" = "1" ]; then
    s="$(echo "$s" | tr -d '0O1lI')"
  fi
  echo "${s:0:$n}"
}

# ===== 下载/解压二进制到临时目录 =====
prepare_binary() {
  local dst="${INSTALL_DIR}/opsmini"
  local tmpdir
  tmpdir="$(mktemp -d)"

  # 确定下载地址：-u 指定 > 默认官方 URL（仅当未指定 -b 本地二进制时）
  local url="$BINARY_URL"
  if [ -z "$url" ] && [ -z "$BINARY" ]; then
    url="${DEFAULT_URL_BASE}/opsmini-${VERSION}-linux-${GOARCH}"
  fi

  if [ -n "$url" ]; then
    info "从 ${url} 下载..."
    local dl="${tmpdir}/download"
    if ! curl -fsSL --connect-timeout 15 -o "$dl" "$url"; then
      error "下载失败: ${url}"
      exit 1
    fi
    case "$(basename "$url")" in
      *.tar.gz|*.tgz)
        tar -xzf "$dl" -C "$tmpdir"
        local found
        found="$(find "$tmpdir" -type f -name 'opsmini*' | grep -E 'opsmini(-[^/]*)?(-linux-${GOARCH})?$' | head -1)"
        # 兼容任意嵌套
        if [ -z "$found" ]; then
          found="$(find "$tmpdir" -type f -name 'opsmini' | head -1)"
        fi
        if [ -z "$found" ]; then
          error "压缩包中未找到 opsmini 二进制"
          exit 1
        fi
        install -m 0755 "$found" "$dst"
        ;;
      *)
        install -m 0755 "$dl" "$dst"
        ;;
    esac
  else
    local src
    src="$(resolve_binary)"
    info "使用二进制: ${src}"
    install -m 0755 "$src" "$dst"
  fi
  rm -rf "$tmpdir"
  info "二进制已安装到 ${dst}"
}

# ===== 生成配置文件 =====
write_config() {
  info "生成配置 ${INSTALL_DIR}/config.yaml（端口 ${PORT}）"
  cat > "${INSTALL_DIR}/config.yaml" <<EOF
# OpsMini 配置文件（由安装脚本生成）
# 说明：JWT 签名密钥与 Agent API Token 在首次启动时自动生成并持久化到数据库；
# AI 大模型在面板「设置 → AI 大模型接入」页面配置。
server:
  host: "${HOST}"
  port: ${PORT}
  secret_entry: ""

database:
  path: "${INSTALL_DIR}/opsmini.db"

jwt:
  access_ttl_seconds: 900
  refresh_ttl_seconds: 604800

log:
  level: error
  path: "${INSTALL_DIR}/opsmini.log"
EOF
  chmod 600 "${INSTALL_DIR}/config.yaml"
}

# ===== 生成初始密码 =====
write_password() {
  local pass
  pass="$(rand_str 16 1)"
  printf '%s\n' "$pass" > "${INSTALL_DIR}/.init_passwd"
  chmod 600 "${INSTALL_DIR}/.init_passwd"
  echo "$pass"
}

# ===== 注册 systemd =====
install_systemd() {
  local password="$1"
  local unit="/etc/systemd/system/opsmini.service"

  info "注册 systemd 服务 ${unit}"
  cat > "$unit" <<EOF
[Unit]
Description=OpsMini Server Panel
After=network.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
ExecStart=${INSTALL_DIR}/opsmini -config ${INSTALL_DIR}/config.yaml
Restart=on-failure
RestartSec=5
# 首次启动用预设密码初始化管理员（已有账号时自动忽略）
Environment=OPSMINI_INIT_PASSWORD=${password}

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable opsmini >/dev/null 2>&1 || warn "enable 失败（可能无 systemd）"
  systemctl restart opsmini >/dev/null 2>&1 || warn "restart 失败（可能无 systemd）"
}

# ===== 主流程 =====
main() {
  echo -e "${C_BOLD}OpsMini 安装${C_RESET}"
  echo "安装目录: ${INSTALL_DIR} | 端口: ${PORT}"

  mkdir -p "$INSTALL_DIR"
  prepare_binary
  write_config

  local password
  if [ -f "${INSTALL_DIR}/opsmini.db" ]; then
    # 已安装（数据库已存在）：seedAdmin 会跳过初始化，OPSMINI_INIT_PASSWORD 不生效，
    # 改用 -reset-pass 重置密码，确保输出的密码能真正登录。
    info "检测到已安装，重置管理员密码..."
    systemctl stop opsmini >/dev/null 2>&1 || true
    sleep 1
    local reset_out
    reset_out="$("${INSTALL_DIR}/opsmini" -config "${INSTALL_DIR}/config.yaml" -reset-pass opsmini 2>&1)"
    password="$(printf '%s\n' "$reset_out" | sed -nE 's/.*密码为：([A-Za-z0-9]+).*/\1/p' | head -1)"
    if [ -z "$password" ]; then
      warn "密码重置失败，使用生成的新密码（若无法登录请手动执行 -reset-pass）"
      password="$(write_password)"
    else
      printf '%s\n' "$password" > "${INSTALL_DIR}/.init_passwd"
      chmod 600 "${INSTALL_DIR}/.init_passwd"
    fi
  else
    password="$(write_password)"
  fi

  if [ "$NO_SYSTEMD" -eq 0 ] && command -v systemctl >/dev/null 2>&1; then
    install_systemd "$password"
  else
    warn "未注册 systemd（手动启动：OPSMINI_INIT_PASSWORD='${password}' ${INSTALL_DIR}/opsmini -config ${INSTALL_DIR}/config.yaml）"
  fi

  # 等待启动并探测
  sleep 2
  if curl -sf "http://127.0.0.1:${PORT}/api/v1/healthz" >/dev/null 2>&1; then
    info "服务已启动并响应"
  else
    warn "服务暂未响应，请稍候检查：systemctl status opsmini 或日志"
  fi

  echo ""
  echo "=============================================================="
  echo -e "  ${C_BOLD}OpsMini 安装完成${C_RESET}"
  echo "=============================================================="
  echo "  访问地址 : http://<服务器IP>:${PORT}"
  echo "  用户名   : opsmini"
  echo -e "  密码     : ${C_BOLD}${password}${C_RESET}"
  echo "  密码文件 : ${INSTALL_DIR}/.init_passwd"
  echo "  配置文件 : ${INSTALL_DIR}/config.yaml"
  echo "  日志文件 : ${INSTALL_DIR}/opsmini.log"
  echo "  服务管理 : systemctl {start|stop|restart|status} opsmini"
  echo "=============================================================="
}

main "$@"
