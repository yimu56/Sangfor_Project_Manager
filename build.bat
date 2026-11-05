@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"
echo ================================================
echo   项目管理 - 编译 Windows 单文件版本
echo ================================================
where go >nul 2>nul
if errorlevel 1 (
  echo [错误] 未检测到 Go 环境，请先安装 Go 1.21+ 并加入 PATH
  pause
  exit /b 1
)
set CGO_ENABLED=0
set GOOS=windows
set GOARCH=amd64
echo 正在编译...
go build -trimpath -ldflags "-s -w" -o "项目管理.exe" .
if errorlevel 1 (
  echo [错误] 编译失败
  pause
  exit /b 1
)
echo.
echo 编译完成: %cd%\项目管理.exe
echo 双击「项目管理.exe」或「启动.bat」即可运行。
echo 浏览器会自动打开 http://127.0.0.1:8787
echo.
pause
