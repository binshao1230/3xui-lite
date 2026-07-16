#!/usr/bin/env bash
set -euo pipefail
INSTALL_DIR="${INSTALL_DIR:-/opt/3xui-lite}"
SERVICE_NAME="3xui-lite"

if [[ "$(id -u)" -ne 0 ]]; then
  echo "请使用 root 运行: sudo bash uninstall.sh"
  exit 1
fi

systemctl stop "${SERVICE_NAME}" 2>/dev/null || true
systemctl disable "${SERVICE_NAME}" 2>/dev/null || true
rm -f "/etc/systemd/system/${SERVICE_NAME}.service"
systemctl daemon-reload

pkill -x xray 2>/dev/null || true
pkill -x sing-box 2>/dev/null || true

read -r -p "是否删除安装目录 ${INSTALL_DIR} 及数据? [y/N] " ans
if [[ "${ans:-}" =~ ^[Yy]$ ]]; then
  rm -rf "$INSTALL_DIR"
  echo "已删除 $INSTALL_DIR"
else
  echo "保留 $INSTALL_DIR"
fi
echo "卸载完成"
