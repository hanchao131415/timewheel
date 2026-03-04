@echo off
REM TimeWheel 一键部署脚本 (Windows)
REM 启动所有基础设施服务 (MySQL + Redis)

setlocal enabledelayedexpansion

set GREEN=[92m
set YELLOW=[93m
set RED=[91m
set NC=[0m

echo ==========================================
echo    TimeWheel 基础设施一键部署
echo ==========================================
echo.

REM 检查 Docker
echo %GREEN%[1/3]%NC% 检查 Docker...
docker --version >nul 2>&1
if errorlevel 1 (
    echo %RED%[ERROR]%NC% Docker 未安装，请先安装 Docker Desktop
    echo 下载地址: https://www.docker.com/products/docker-desktop
    pause
    exit /b 1
)
echo       Docker 已安装

REM 启动服务
echo.
echo %GREEN%[2/3]%NC% 启动基础设施服务...
docker-compose -f deployments\docker-compose.infra.yaml up -d

if errorlevel 1 (
    echo %RED%[ERROR]%NC% 启动失败
    pause
    exit /b 1
)

echo.
echo %GREEN%[3/3]%NC% 等待服务就绪...
timeout /t 10 /nobreak >nul

REM 显示服务状态
echo.
echo ==========================================
echo    服务状态
echo ==========================================
docker-compose -f deployments\docker-compose.infra.yaml ps

echo.
echo ==========================================
echo    连接信息
echo ==========================================
echo.
echo MySQL:
echo   Host: localhost
echo   Port: 3306
echo   Database: timewheel
echo   User: timewheel
echo   Password: timewheel123
echo   Root Password: timewheel123
echo   Connection String: timewheel:timewheel123@tcp(localhost:3306)/timewheel?charset=utf8mb4^&parseTime=True^&loc=Local
echo.
echo Redis:
echo   Host: localhost
echo   Port: 6379
echo   Password: timewheel123
echo   Connection String: redis://:timewheel123@localhost:6379/0
echo.
echo RabbitMQ:
echo   Host: localhost
echo   AMQP Port: 5672
echo   Management UI: http://localhost:15672
echo   Username: timewheel
echo   Password: timewheel123
echo   Virtual Host: /timewheel
echo   Connection URL: amqp://timewheel:timewheel123@localhost:5672/timewheel
echo.
echo ==========================================
echo    管理命令
echo ==========================================
echo.
echo   停止所有服务:  docker-compose -f deployments\docker-compose.infra.yaml down
echo   查看日志:      docker-compose -f deployments\docker-compose.infra.yaml logs -f
echo   重启服务:      docker-compose -f deployments\docker-compose.infra.yaml restart
echo.
echo   MySQL 独立管理: scripts\deploy-mysql.bat [start^|stop^|status]
echo   Redis 独立管理: scripts\deploy-redis.bat [start^|stop^|status]
echo   RabbitMQ 独立管理: scripts\deploy-rabbitmq.bat [start^|stop^|status]
echo.
echo 延迟队列插件启用 (首次启动后执行):
echo   docker exec timewheel-rabbitmq rabbitmq-plugins enable rabbitmq_delayed_message_exchange
echo.

pause
