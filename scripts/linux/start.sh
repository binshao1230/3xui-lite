#!/usr/bin/env bash
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$DIR"

export XUI_LISTEN="${XUI_LISTEN:-0.0.0.0:18080}"
export XUI_DATA="${XUI_DATA:-$DIR/data}"
export XRAY_BIN="${XRAY_BIN:-$DIR/bin/xray}"
export SINGBOX_BIN="${SINGBOX_BIN:-$DIR/bin/sing-box}"

mkdir -p "$XUI_DATA"
chmod +x "$DIR/3xui-lite" "$XRAY_BIN" "$SINGBOX_BIN" 2>/dev/null || true

if pgrep -f "$DIR/3xui-lite" >/dev/null 2>&1; then
  echo "3xui-lite 已在运行"
  exit 0
fi

echo "启动 3xui-lite ..."
echo "面板: http://服务器IP:18080"
echo "默认账号: admin / admin"
nohup "$DIR/3xui-lite" >>"$XUI_DATA/panel.log" 2>&1 &
echo $! >"$XUI_DATA/panel.pid"
sleep 1
if kill -0 "$(cat "$XUI_DATA/panel.pid")" 2>/dev/null; then
  echo "已启动 PID=$(cat "$XUI_DATA/panel.pid")"
else
  echo "启动失败，查看 $XUI_DATA/panel.log"
  exit 1
fi
