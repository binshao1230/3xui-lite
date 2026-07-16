#!/usr/bin/env bash
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
DATA="${XUI_DATA:-$DIR/data}"

if [[ -f "$DATA/panel.pid" ]]; then
  PID="$(cat "$DATA/panel.pid" || true)"
  if [[ -n "${PID:-}" ]] && kill -0 "$PID" 2>/dev/null; then
    kill "$PID" 2>/dev/null || true
    sleep 1
    kill -9 "$PID" 2>/dev/null || true
  fi
  rm -f "$DATA/panel.pid"
fi

# stop cores started by panel
pkill -x xray 2>/dev/null || true
pkill -x sing-box 2>/dev/null || true
# also match by path if names differ
pkill -f "$DIR/bin/xray" 2>/dev/null || true
pkill -f "$DIR/bin/sing-box" 2>/dev/null || true
pkill -f "$DIR/3xui-lite" 2>/dev/null || true

echo "已停止 3xui-lite / xray / sing-box"
