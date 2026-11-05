@echo off
chcp 65001 >nul
cd /d "%~dp0"
if exist "项目管理.exe" (
  start "" "项目管理.exe"
) else (
  echo 未找到 项目管理.exe，正在尝试编译...
  call build.bat
)
