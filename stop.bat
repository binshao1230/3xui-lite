@echo off
chcp 65001 >nul
echo 正在停止 3xui-lite 与 xray ...
taskkill /IM 3xui-lite.exe /F >nul 2>&1
taskkill /IM xray.exe /F >nul 2>&1
taskkill /IM sing-box.exe /F >nul 2>&1
echo 已停止。
timeout /t 2 >nul
