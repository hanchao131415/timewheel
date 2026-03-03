@echo off
REM TimeWheel 停止所有服务脚本 (Windows)

setlocal enabledelayedexpansion

set GREEN=[92m
set YELLOW=[93m
set RED=[91m
set NC=[0m

echo ==========================================
echo    停止 TimeWheel 基础设施服务
echo ==========================================
echo.

echo %GREEN%[INFO]%NC% 停止所有服务...
docker-compose -f deployments\docker-compose.infra.yaml down

if errorlevel 1 (
    echo %RED%[ERROR]%NC% 停止失败
    pause
    exit /b 1
)

echo.
echo %GREEN%[INFO]%NC% 所有服务已停止
echo.
echo 如需删除数据卷，请运行:
echo   docker-compose -f deployments\docker-compose.infra.yaml down -v
echo.

pause
