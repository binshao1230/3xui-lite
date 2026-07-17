#!/usr/bin/env bash
# 本地包内安装（已解压 3xui-lite-linux-amd64 后执行）
# sudo bash install.sh
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/3xui-lite}"
SERVICE_NAME="3xui-lite"
LISTEN="${XUI_LISTEN:-0.0.0.0:18080}"
SRC="$(cd "$(dirname "$0")" && pwd)"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "请使用 root: sudo bash install.sh"
  exit 1
fi

echo "==> 从本地包安装到 ${INSTALL_DIR}"
mkdir -p "$INSTALL_DIR" "$INSTALL_DIR/bin" "$INSTALL_DIR/data"

echo "==> 停止旧进程（避免 Text file busy）"
systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
pkill -x xray 2>/dev/null || true
pkill -x sing-box 2>/dev/null || true
pkill -f "$INSTALL_DIR/3xui-lite" 2>/dev/null || true
sleep 1

# 安全替换正在运行的二进制
safe_mv() {
  local src="$1" dst="$2"
  local tmp="${dst}.new.$$"
  cp -f "$src" "$tmp"
  chmod +x "$tmp" 2>/dev/null || true
  mv -f "$tmp" "$dst"
}

safe_mv "$SRC/3xui-lite" "$INSTALL_DIR/3xui-lite"
for f in xray sing-box geoip.dat geosite.dat; do
  if [[ -f "$SRC/bin/$f" ]]; then
    if [[ "$f" == "xray" || "$f" == "sing-box" ]]; then
      safe_mv "$SRC/bin/$f" "$INSTALL_DIR/bin/$f"
    else
      cp -f "$SRC/bin/$f" "$INSTALL_DIR/bin/$f"
    fi
  fi
done
for f in start.sh stop.sh uninstall.sh README-VPS.md; do
  [[ -f "$SRC/$f" ]] && cp -f "$SRC/$f" "$INSTALL_DIR/$f"
done
chmod +x "$INSTALL_DIR/3xui-lite" "$INSTALL_DIR/bin/xray" "$INSTALL_DIR/bin/sing-box" 2>/dev/null || true
chmod +x "$INSTALL_DIR/"*.sh 2>/dev/null || true

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
systemctl --no-pager --full status "${SERVICE_NAME}" || true

IP="$(curl -fsSL --connect-timeout 3 https://api.ipify.org 2>/dev/null || hostname -I 2>/dev/null | awk '{print $1}')"
echo ""
echo "============================================"
echo " 安装完成"
echo " 面板: http://${IP:-服务器IP}:18080"
echo " 账号: admin / admin"
echo " 目录: $INSTALL_DIR"
echo "============================================"
