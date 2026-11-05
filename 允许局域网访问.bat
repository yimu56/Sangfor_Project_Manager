@echo off
chcp 65001 >nul
cd /d "%~dp0"

rem 需要管理员权限：自动提权
net session >nul 2>&1
if errorlevel 1 (
  echo 请求管理员权限...
  powershell -NoProfile -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
  exit /b
)

set PORT=8787
set RULE=项目管理-PM%PORT%

echo ================================================
echo   放行 Windows 防火墙入站端口 TCP %PORT%
echo ================================================
netsh advfirewall firewall delete rule name="%RULE%" >nul 2>&1
netsh advfirewall firewall add rule name="%RULE%" dir=in action=allow protocol=TCP localport=%PORT%
if errorlevel 1 (
  echo [失败] 规则添加失败，请确认是否以管理员身份运行。
) else (
  echo.
  echo [成功] 已放行 TCP %PORT%，局域网内其他主机现在可以访问。
  echo 如需撤销，运行本脚本后执行：
  echo    netsh advfirewall firewall delete rule name="%RULE%"
  echo 提示：若启动时改了端口（-port 9000），请把脚本里的 PORT 改成对应端口再运行。
)
echo.
pause
