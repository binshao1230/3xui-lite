#!/usr/bin/env bash
# =============================================================================
# 3xui-lite 一键安装脚本 (Linux)
# 用法:
#   curl -fsSL https://raw.githubusercontent.com/binshao1230/3xui-lite/main/install.sh | sudo bash
# 或:
#   wget -qO- https://raw.githubusercontent.com/binshao1230/3xui-lite/main/install.sh | sudo bash
#
# 可选环境变量:
#   INSTALL_DIR=/opt/3xui-lite
#   XUI_LISTEN=0.0.0.0:18080
#   XUI_VERSION=latest          # 或 v0.2.0 等 tag
#   REPO=binshao1230/3xui-lite
# =============================================================================
set -euo pipefail

REPO="${REPO:-binshao1230/3xui-lite}"
INSTALL_DIR="${INSTALL_DIR:-/opt/3xui-lite}"
LISTEN="${XUI_LISTEN:-0.0.0.0:18080}"
SERVICE_NAME="3xui-lite"
VERSION="${XUI_VERSION:-latest}"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

red()    { echo -e "\033[31m$*\033[0m"; }
green()  { echo -e "\033[32m$*\033[0m"; }
yellow() { echo -e "\033[33m$*\033[0m"; }
info()   { echo -e "\033[36m[INFO]\033[0m $*"; }
ok()     { green "[ OK ] $*"; }
fail()   { red "[FAIL] $*"; exit 1; }

need_root() {
  if [[ "$(id -u)" -ne 0 ]]; then
    fail "请使用 root 运行。示例: curl -fsSL https://raw.githubusercontent.com/${REPO}/main/install.sh | sudo bash"
  fi
}

detect_arch() {
  local m
  m="$(uname -m)"
  case "$m" in
    x86_64|amd64) ARCH="amd64"; XRAY_ASSET="Xray-linux-64.zip"; SB_PATTERN="linux-amd64.tar.gz" ;;
    aarch64|arm64) ARCH="arm64"; XRAY_ASSET="Xray-linux-arm64-v8a.zip"; SB_PATTERN="linux-arm64.tar.gz" ;;
    *) fail "暂不支持架构: $m （仅支持 amd64 / arm64）" ;;
  esac
  ok "架构: $ARCH"
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || return 1
}

ensure_deps() {
  local miss=()
  for c in curl tar unzip; do
    need_cmd "$c" || miss+=("$c")
  done
  # systemctl optional check later
  if [[ ${#miss[@]} -eq 0 ]]; then
    ok "依赖齐全"
    return 0
  fi
  info "安装依赖: ${miss[*]}"
  if need_cmd apt-get; then
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -y
    apt-get install -y curl tar unzip ca-certificates
  elif need_cmd dnf; then
    dnf install -y curl tar unzip ca-certificates
  elif need_cmd yum; then
    yum install -y curl tar unzip ca-certificates
  elif need_cmd apk; then
    apk add --no-cache curl tar unzip ca-certificates
  else
    fail "缺少依赖: ${miss[*]}，请手动安装后重试"
  fi
  ok "依赖安装完成"
}

download() {
  local url="$1" out="$2"
  info "下载: $url"
  if ! curl -fL --retry 3 --connect-timeout 15 -o "$out" "$url"; then
    fail "下载失败: $url"
  fi
}

# 从 GitHub Release 下载预打包（含面板+内核）
try_release_package() {
  local api tag url name asset
  info "尝试从 GitHub Release 获取安装包..."
  if [[ "$VERSION" == "latest" ]]; then
    api="https://api.github.com/repos/${REPO}/releases/latest"
  else
    api="https://api.github.com/repos/${REPO}/releases/tags/${VERSION}"
  fi
  if ! curl -fsSL "$api" -o "$TMP_DIR/release.json" 2>/dev/null; then
    yellow "未找到 Release，将改为在线组装安装"
    return 1
  fi
  # 优先精确名称
  name="3xui-lite-linux-${ARCH}.tar.gz"
  asset="$(grep -oE "\"browser_download_url\": \"[^\"]+${name}\"" "$TMP_DIR/release.json" | head -1 | sed 's/.*"\(https[^\"]*\)".*/\1/')"
  if [[ -z "$asset" ]]; then
    # 兼容任意含 linux 与 arch 的包
    asset="$(grep -oE "\"browser_download_url\": \"[^\"]+linux[^\"]*${ARCH}[^\"]*\.tar\.gz\"" "$TMP_DIR/release.json" | head -1 | sed 's/.*"\(https[^\"]*\)".*/\1/')"
  fi
  if [[ -z "$asset" ]]; then
    yellow "Release 中无 linux-${ARCH} 包，改为在线组装"
    return 1
  fi
  download "$asset" "$TMP_DIR/pkg.tar.gz"
  tar -xzf "$TMP_DIR/pkg.tar.gz" -C "$TMP_DIR"
  # 找到解压出的目录（兼容 Windows 打包无 +x 权限）
  if [[ -d "$TMP_DIR/3xui-lite-linux-${ARCH}" ]]; then
    PKG_DIR="$TMP_DIR/3xui-lite-linux-${ARCH}"
  elif [[ -f "$TMP_DIR/3xui-lite" ]]; then
    PKG_DIR="$TMP_DIR"
  else
    local found
    found="$(find "$TMP_DIR" -maxdepth 3 -type f -name '3xui-lite' 2>/dev/null | head -1 || true)"
    if [[ -n "$found" ]]; then
      PKG_DIR="$(cd "$(dirname "$found")" && pwd)"
    fi
  fi
  if [[ -z "${PKG_DIR:-}" || ! -f "$PKG_DIR/3xui-lite" ]]; then
    yellow "安装包结构异常，目录列表:"
    find "$TMP_DIR" -maxdepth 3 -type f 2>/dev/null | head -30 || true
    fail "安装包结构异常（未找到 3xui-lite 二进制）"
  fi
  # Windows 打的 tar 常无可执行位，安装前强制 chmod
  chmod +x "$PKG_DIR/3xui-lite" 2>/dev/null || true
  chmod +x "$PKG_DIR/bin/xray" "$PKG_DIR/bin/sing-box" 2>/dev/null || true
  chmod +x "$PKG_DIR/"*.sh 2>/dev/null || true
  # 再校验文件存在且非空
  [[ -s "$PKG_DIR/3xui-lite" ]] || fail "面板二进制为空: $PKG_DIR/3xui-lite"
  [[ -f "$PKG_DIR/bin/xray" || -f "$PKG_DIR/bin/xray.exe" ]] || yellow "警告: 包内未找到 xray，将尝试在线补齐"
  ok "已获取预编译安装包: $PKG_DIR"
  return 0
}

# 在线组装：下载面板二进制（或从源码构建）+ xray + sing-box
assemble_online() {
  info "在线组装安装文件..."
  PKG_DIR="$TMP_DIR/assemble"
  mkdir -p "$PKG_DIR/bin" "$PKG_DIR/data"

  # 1) 面板二进制：优先 release 中的 3xui-lite-linux-${ARCH} 裸文件
  local api panel_url
  if [[ "$VERSION" == "latest" ]]; then
    api="https://api.github.com/repos/${REPO}/releases/latest"
  else
    api="https://api.github.com/repos/${REPO}/releases/tags/${VERSION}"
  fi
  panel_url=""
  if curl -fsSL "$api" -o "$TMP_DIR/rel2.json" 2>/dev/null; then
    panel_url="$(grep -oE "\"browser_download_url\": \"[^\"]*3xui-lite-linux-${ARCH}[^\"]*\"" "$TMP_DIR/rel2.json" | grep -v tar.gz | head -1 | sed 's/.*"\(https[^\"]*\)".*/\1/')"
  fi
  if [[ -n "$panel_url" ]]; then
    download "$panel_url" "$PKG_DIR/3xui-lite"
  elif need_cmd go; then
    info "本机有 Go，从源码编译面板..."
    download "https://codeload.github.com/${REPO}/tar.gz/refs/heads/main" "$TMP_DIR/src.tar.gz"
    tar -xzf "$TMP_DIR/src.tar.gz" -C "$TMP_DIR"
    local src
    src="$(find "$TMP_DIR" -maxdepth 1 -type d -name '3xui-lite-*' | head -1)"
    (cd "$src" && CGO_ENABLED=0 GOOS=linux GOARCH="$ARCH" go build -ldflags "-s -w" -o "$PKG_DIR/3xui-lite" ./cmd/server)
  else
    # 最后尝试直接拉 main 源码 + 安装 go 太重，提示用户
    fail "无法获取面板二进制。请先发布 Release 包 3xui-lite-linux-${ARCH}.tar.gz，或安装 Go 后重试"
  fi
  chmod +x "$PKG_DIR/3xui-lite"

  # 2) Xray
  info "下载 Xray-core..."
  local xray_ver xray_url
  xray_ver="$(curl -fsSL https://api.github.com/repos/XTLS/Xray-core/releases/latest | grep -oE '"tag_name":\s*"[^"]+"' | head -1 | cut -d'"' -f4)"
  [[ -n "$xray_ver" ]] || fail "获取 Xray 版本失败"
  xray_url="https://github.com/XTLS/Xray-core/releases/download/${xray_ver}/${XRAY_ASSET}"
  download "$xray_url" "$TMP_DIR/xray.zip"
  unzip -qo "$TMP_DIR/xray.zip" -d "$TMP_DIR/xray"
  cp -f "$TMP_DIR/xray/xray" "$PKG_DIR/bin/xray"
  cp -f "$TMP_DIR/xray/geoip.dat" "$PKG_DIR/bin/" 2>/dev/null || true
  cp -f "$TMP_DIR/xray/geosite.dat" "$PKG_DIR/bin/" 2>/dev/null || true
  chmod +x "$PKG_DIR/bin/xray"

  # 3) sing-box
  info "下载 sing-box..."
  local sb_ver sb_name sb_url
  sb_ver="$(curl -fsSL https://api.github.com/repos/SagerNet/sing-box/releases/latest | grep -oE '"tag_name":\s*"[^"]+"' | head -1 | cut -d'"' -f4)"
  [[ -n "$sb_ver" ]] || fail "获取 sing-box 版本失败"
  sb_name="sing-box-${sb_ver#v}-linux-${ARCH}.tar.gz"
  sb_url="https://github.com/SagerNet/sing-box/releases/download/${sb_ver}/${sb_name}"
  download "$sb_url" "$TMP_DIR/sb.tar.gz"
  tar -xzf "$TMP_DIR/sb.tar.gz" -C "$TMP_DIR"
  local sb_bin
  sb_bin="$(find "$TMP_DIR" -type f -name sing-box | head -1)"
  [[ -n "$sb_bin" ]] || fail "sing-box 二进制未找到"
  cp -f "$sb_bin" "$PKG_DIR/bin/sing-box"
  chmod +x "$PKG_DIR/bin/sing-box"

  ok "在线组装完成"
}

# 安全替换正在运行的二进制（避免 Text file busy）
safe_install_bin() {
  local src="$1" dst="$2"
  [[ -f "$src" ]] || return 0
  mkdir -p "$(dirname "$dst")"
  # 先写到临时文件再 mv（即使目标正在运行，Linux 也可 unlink/mv 覆盖路径）
  local tmp="${dst}.new.$$"
  cp -f "$src" "$tmp"
  chmod +x "$tmp" 2>/dev/null || true
  mv -f "$tmp" "$dst"
}

stop_running_panel() {
  info "停止正在运行的面板/内核（避免 Text file busy）..."
  if need_cmd systemctl; then
    systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
    systemctl stop "${SERVICE_NAME}.service" 2>/dev/null || true
  fi
  # nohup / 残留进程
  if [[ -f "$INSTALL_DIR/data/panel.pid" ]]; then
    kill "$(cat "$INSTALL_DIR/data/panel.pid" 2>/dev/null)" 2>/dev/null || true
    rm -f "$INSTALL_DIR/data/panel.pid"
  fi
  pkill -x xray 2>/dev/null || true
  pkill -x sing-box 2>/dev/null || true
  # 按路径杀面板（避免误杀同名无关进程：尽量精确）
  if [[ -x "$INSTALL_DIR/3xui-lite" ]]; then
    pkill -f "$INSTALL_DIR/3xui-lite" 2>/dev/null || true
  fi
  pkill -x 3xui-lite 2>/dev/null || true
  sleep 1
  # 仍占用则强杀
  if pgrep -f "$INSTALL_DIR/3xui-lite" >/dev/null 2>&1; then
    pkill -9 -f "$INSTALL_DIR/3xui-lite" 2>/dev/null || true
    sleep 1
  fi
}

install_files() {
  info "安装到 $INSTALL_DIR"
  mkdir -p "$INSTALL_DIR" "$INSTALL_DIR/bin" "$INSTALL_DIR/data"
  if [[ -d "$INSTALL_DIR/data" ]]; then
    info "保留已有数据目录 data/"
  fi

  # 必须先停服务，否则 cp 面板二进制会 Text file busy
  stop_running_panel

  # 面板主程序
  if [[ -f "$PKG_DIR/3xui-lite" ]]; then
    safe_install_bin "$PKG_DIR/3xui-lite" "$INSTALL_DIR/3xui-lite"
  else
    fail "安装包中没有 3xui-lite 二进制"
  fi

  # 内核与 geo 数据
  mkdir -p "$INSTALL_DIR/bin"
  for f in xray sing-box geoip.dat geosite.dat; do
    if [[ -f "$PKG_DIR/bin/$f" ]]; then
      if [[ "$f" == "xray" || "$f" == "sing-box" ]]; then
        safe_install_bin "$PKG_DIR/bin/$f" "$INSTALL_DIR/bin/$f"
      else
        cp -f "$PKG_DIR/bin/$f" "$INSTALL_DIR/bin/$f"
      fi
    fi
  done

  # 其它脚本/说明（不覆盖 data）
  for f in start.sh stop.sh uninstall.sh README-VPS.md 3xui-lite.service online-install.sh; do
    [[ -f "$PKG_DIR/$f" ]] && cp -f "$PKG_DIR/$f" "$INSTALL_DIR/$f"
  done

  chmod +x "$INSTALL_DIR/3xui-lite" 2>/dev/null || true
  chmod +x "$INSTALL_DIR/bin/xray" "$INSTALL_DIR/bin/sing-box" 2>/dev/null || true
  chmod +x "$INSTALL_DIR/"*.sh 2>/dev/null || true

  # 若内核缺失则在线补齐
  if [[ ! -f "$INSTALL_DIR/bin/xray" ]]; then
    yellow "补齐 xray..."
    local xray_ver xray_url
    xray_ver="$(curl -fsSL https://api.github.com/repos/XTLS/Xray-core/releases/latest | grep -oE '"tag_name":\s*"[^"]+"' | head -1 | cut -d'"' -f4)"
    xray_url="https://github.com/XTLS/Xray-core/releases/download/${xray_ver}/${XRAY_ASSET}"
    download "$xray_url" "$TMP_DIR/xray-fix.zip"
    unzip -qo "$TMP_DIR/xray-fix.zip" -d "$TMP_DIR/xray-fix"
    safe_install_bin "$TMP_DIR/xray-fix/xray" "$INSTALL_DIR/bin/xray"
    cp -f "$TMP_DIR/xray-fix/geoip.dat" "$INSTALL_DIR/bin/" 2>/dev/null || true
    cp -f "$TMP_DIR/xray-fix/geosite.dat" "$INSTALL_DIR/bin/" 2>/dev/null || true
  fi
  if [[ ! -f "$INSTALL_DIR/bin/sing-box" ]]; then
    yellow "补齐 sing-box..."
    local sb_ver sb_name sb_url sb_bin
    sb_ver="$(curl -fsSL https://api.github.com/repos/SagerNet/sing-box/releases/latest | grep -oE '"tag_name":\s*"[^"]+"' | head -1 | cut -d'"' -f4)"
    sb_name="sing-box-${sb_ver#v}-linux-${ARCH}.tar.gz"
    sb_url="https://github.com/SagerNet/sing-box/releases/download/${sb_ver}/${sb_name}"
    download "$sb_url" "$TMP_DIR/sb-fix.tar.gz"
    tar -xzf "$TMP_DIR/sb-fix.tar.gz" -C "$TMP_DIR"
    sb_bin="$(find "$TMP_DIR" -type f -name sing-box | head -1)"
    safe_install_bin "$sb_bin" "$INSTALL_DIR/bin/sing-box"
  fi
  chmod +x "$INSTALL_DIR/3xui-lite" "$INSTALL_DIR/bin/xray" "$INSTALL_DIR/bin/sing-box"

  # helper scripts
  cat >"$INSTALL_DIR/start.sh" <<'EOS'
#!/usr/bin/env bash
systemctl start 3xui-lite
systemctl status 3xui-lite --no-pager || true
EOS
  cat >"$INSTALL_DIR/stop.sh" <<'EOS'
#!/usr/bin/env bash
systemctl stop 3xui-lite
pkill -x xray 2>/dev/null || true
pkill -x sing-box 2>/dev/null || true
echo stopped
EOS
  cat >"$INSTALL_DIR/uninstall.sh" <<EOF
#!/usr/bin/env bash
set -euo pipefail
systemctl stop ${SERVICE_NAME} 2>/dev/null || true
systemctl disable ${SERVICE_NAME} 2>/dev/null || true
rm -f /etc/systemd/system/${SERVICE_NAME}.service
systemctl daemon-reload
pkill -x xray 2>/dev/null || true
pkill -x sing-box 2>/dev/null || true
read -r -p "删除 ${INSTALL_DIR} 及数据? [y/N] " a
[[ "\${a:-}" =~ ^[Yy]\$ ]] && rm -rf "${INSTALL_DIR}"
echo "卸载完成"
EOF
  chmod +x "$INSTALL_DIR/start.sh" "$INSTALL_DIR/stop.sh" "$INSTALL_DIR/uninstall.sh"
  ok "文件已安装"
}

install_systemd() {
  if ! need_cmd systemctl; then
    yellow "未检测到 systemd，将使用 nohup 后台启动"
    (cd "$INSTALL_DIR" && \
      XUI_LISTEN="$LISTEN" XUI_DATA="$INSTALL_DIR/data" \
      XRAY_BIN="$INSTALL_DIR/bin/xray" SINGBOX_BIN="$INSTALL_DIR/bin/sing-box" \
      nohup ./3xui-lite >>"$INSTALL_DIR/data/panel.log" 2>&1 & echo $! >"$INSTALL_DIR/data/panel.pid")
    ok "已 nohup 启动"
    return 0
  fi
  info "配置 systemd 服务..."
  cat >"/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=3xui-lite panel (Xray / sing-box)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=${INSTALL_DIR}
Environment=XUI_LISTEN=${LISTEN}
Environment=XUI_DATA=${INSTALL_DIR}/data
Environment=XRAY_BIN=${INSTALL_DIR}/bin/xray
Environment=SINGBOX_BIN=${INSTALL_DIR}/bin/sing-box
ExecStart=${INSTALL_DIR}/3xui-lite
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576

[Install]
WantedBy=multi-user.target
EOF
  systemctl daemon-reload
  systemctl enable "${SERVICE_NAME}"
  systemctl restart "${SERVICE_NAME}"
  sleep 1
  if systemctl is-active --quiet "${SERVICE_NAME}"; then
    ok "服务已启动: ${SERVICE_NAME}"
  else
    yellow "服务可能未就绪，查看: journalctl -u ${SERVICE_NAME} -n 50 --no-pager"
    systemctl --no-pager --full status "${SERVICE_NAME}" || true
  fi
}

open_firewall_hint() {
  if need_cmd ufw && ufw status 2>/dev/null | grep -q "Status: active"; then
    info "检测到 ufw，尝试放行 18080..."
    ufw allow 18080/tcp >/dev/null 2>&1 || true
  fi
}

print_done() {
  local ip
  ip="$(curl -fsSL --connect-timeout 3 https://api.ipify.org 2>/dev/null || true)"
  [[ -z "$ip" ]] && ip="$(hostname -I 2>/dev/null | awk '{print $1}')"
  [[ -z "$ip" ]] && ip="你的服务器IP"
  echo ""
  green "============================================"
  green "  3xui-lite 安装完成"
  green "============================================"
  echo "  面板地址: http://${ip}:18080"
  echo "  默认账号: admin"
  echo "  默认密码: admin   ← 请立即修改"
  echo "  安装目录: ${INSTALL_DIR}"
  echo ""
  echo "  常用命令:"
  echo "    systemctl status 3xui-lite"
  echo "    systemctl restart 3xui-lite"
  echo "    journalctl -u 3xui-lite -f"
  echo "    bash ${INSTALL_DIR}/uninstall.sh"
  echo ""
  echo "  云安全组 / 防火墙请放行: 18080 及入站端口(如 8443)"
  green "============================================"
}

main() {
  echo ""
  green ">>> 3xui-lite 一键安装"
  need_root
  detect_arch
  ensure_deps
  if try_release_package; then
    :
  else
    assemble_online
  fi
  install_files
  install_systemd
  open_firewall_hint
  print_done
}

main "$@"
