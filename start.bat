@echo off
chcp 65001 >nul
cd /d "%~dp0"

set "XUI_LISTEN=127.0.0.1:18080"
set "XUI_DATA=%~dp0data"
set "XRAY_BIN=%~dp0bin\xray.exe"
set "SINGBOX_BIN=%~dp0bin\sing-box.exe"

if not exist "%~dp03xui-lite.exe" (
  echo [错误] 找不到 3xui-lite.exe
  pause
  exit /b 1
)
if not exist "%XRAY_BIN%" (
  echo [错误] 找不到 xray: %XRAY_BIN%
  pause
  exit /b 1
)

:: 若已在运行则只打开浏览器
netstat -ano | findstr ":18080" | findstr "LISTENING" >nul 2>&1
if %errorlevel%==0 (
  echo 面板已在运行，打开浏览器...
  start "" "http://127.0.0.1:18080/"
  exit /b 0
)

echo 正在启动 3xui-lite ...
echo 地址: http://127.0.0.1:18080
echo 账号: admin / admin
echo.
echo 请保持本窗口打开；关闭窗口会停止面板。
echo ----------------------------------------
start "" "http://127.0.0.1:18080/"
"%~dp03xui-lite.exe"
echo.
echo 面板已退出，代码 %ERRORLEVEL%
pause
