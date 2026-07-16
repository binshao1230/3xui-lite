#!/usr/bin/env bash
# 3xui-lite VPS 安装脚本（systemd）
set -euo pipefail

INSTALL_DIR="${INSTALL_DIR:-/opt/3xui-lite}"
SERVICE_NAME="3xui-lite"
LISTEN="${XUI_LISTEN:-0.0.0.0:18080}"

SRC="$(cd "$(dirname "$0")" && pwd)"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "请使用 root 运行: sudo bash install.sh"
  exit 1
fi

echo "==> 安装目录: $INSTALL_DIR"
mkdir -p "$INSTALL_DIR"
# 复制程序与内核（不覆盖已有 data）
rsync -a --exclude 'data' "$SRC/" "$INSTALL_DIR/" 2>/dev/null || {
  # fallback without rsync
  cp -a "$SRC/3xui-lite" "$INSTALL_DIR/"
  cp -a "$SRC/bin" "$INSTALL_DIR/"
  cp -a "$SRC/"*.sh "$INSTALL_DIR/" 2>/dev/null || true
  cp -a "$SRC/3xui-lite.service" "$INSTALL_DIR/" 2>/dev/null || true
  cp -a "$SRC/README-VPS.md" "$INSTALL_DIR/" 2>/dev/null || true
}
mkdir -p "$INSTALL_DIR/data"
chmod +x "$INSTALL_DIR/3xui-lite" "$INSTALL_DIR/bin/xray" "$INSTALL_DIR/bin/sing-box" \
  "$INSTALL_DIR/start.sh" "$INSTALL_DIR/stop.sh" "$INSTALL_DIR/install.sh" 2>/dev/null || true

# systemd unit
cat >"/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=3xui-lite panel (Xray / sing-box)
After=network.target

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

# firewall hints
echo ""
echo "============================================"
echo " 安装完成"
echo " 面板地址: http://服务器公网IP:18080"
echo " 默认账号: admin / admin  （请立即修改）"
echo " 安装路径: $INSTALL_DIR"
echo ""
echo " 常用命令:"
echo "   systemctl status 3xui-lite"
echo "   systemctl restart 3xui-lite"
echo "   journalctl -u 3xui-lite -f"
echo ""
echo " 若无法访问，请放行端口，例如:"
echo "   ufw allow 18080/tcp"
echo "   ufw allow 8443/tcp   # 入站示例端口"
echo "============================================"
