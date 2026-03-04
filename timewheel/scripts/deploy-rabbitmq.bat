@echo off
REM RabbitMQ 部署脚本 for TimeWheel (Windows)
REM 支持延迟队列功能
REM 用法: deploy-rabbitmq.bat [start|stop|restart|status|logs]

setlocal enabledelayedexpansion

REM 配置变量
set CONTAINER_NAME=timewheel-rabbitmq
set RABBITMQ_PORT=5672
set RABBITMQ_MGMT_PORT=15672
set RABBITMQ_USER=timewheel
set RABBITMQ_PASSWORD=timewheel123
set RABBITMQ_VHOST=/timewheel
set DATA_DIR=.\data\rabbitmq
set RABBITMQ_VERSION=3.13-management

REM 颜色代码 (Windows 10+)
set GREEN=[92m
set YELLOW=[93m
set RED=[91m
set NC=[0m

:main
if "%1"=="" goto usage
if "%1"=="start" goto start
if "%1"=="stop" goto stop
if "%1"=="restart" goto restart
if "%1"=="status" goto status
if "%1"=="logs" goto logs
if "%1"=="remove" goto remove
if "%1"=="create-delay-queue" goto create_delay_queue
if "%1"=="create-queue" goto create_queue
if "%1"=="list" goto list
goto usage

:start
echo %GREEN%[INFO]%NC% 启动 RabbitMQ (带延迟队列插件)...

REM 检查 Docker
docker --version >nul 2>&1
if errorlevel 1 (
    echo %RED%[ERROR]%NC% Docker 未安装，请先安装 Docker Desktop
    exit /b 1
)

REM 检查容器是否已存在
docker ps -a --format "{{.Names}}" | findstr /x "%CONTAINER_NAME%" >nul
if not errorlevel 1 (
    docker ps --format "{{.Names}}" | findstr /x "%CONTAINER_NAME%" >nul
    if not errorlevel 1 (
        echo %YELLOW%[WARN]%NC% RabbitMQ 容器已在运行中
        exit /b 0
    )
    echo %GREEN%[INFO]%NC% 启动已存在的 RabbitMQ 容器...
    docker start %CONTAINER_NAME%
    exit /b 0
)

REM 创建数据目录
if not exist "%DATA_DIR%" mkdir "%DATA_DIR%"

REM 启动容器
echo %GREEN%[INFO]%NC% 启动 RabbitMQ 容器...
docker run -d ^
    --name %CONTAINER_NAME% ^
    --restart=unless-stopped ^
    -p %RABBITMQ_PORT%:5672 ^
    -p %RABBITMQ_MGMT_PORT%:15672 ^
    -e RABBITMQ_DEFAULT_USER=%RABBITMQ_USER% ^
    -e RABBITMQ_DEFAULT_PASS=%RABBITMQ_PASSWORD% ^
    -e RABBITMQ_DEFAULT_VHOST=%RABBITMQ_VHOST% ^
    -v %cd%\%DATA_DIR%:/var/lib/rabbitmq ^
    rabbitmq:%RABBITMQ_VERSION%

echo %GREEN%[INFO]%NC% 等待 RabbitMQ 启动...
timeout /t 15 /nobreak >nul

REM 安装延迟队列插件
echo %GREEN%[INFO]%NC% 安装延迟队列插件...
docker exec %CONTAINER_NAME% rabbitmq-plugins enable rabbitmq_delayed_message_exchange

REM 重启以加载插件
echo %GREEN%[INFO]%NC% 重启容器以加载插件...
docker restart %CONTAINER_NAME%
timeout /t 10 /nobreak >nul

REM 健康检查
echo %GREEN%[INFO]%NC% 检查 RabbitMQ 状态...
set /a count=0
:healthcheck
set /a count+=1
if %count% gtr 30 (
    echo %RED%[ERROR]%NC% RabbitMQ 启动超时
    exit /b 1
)
docker exec %CONTAINER_NAME% rabbitmqctl status >nul 2>&1
if errorlevel 1 (
    timeout /t 1 /nobreak >nul
    goto healthcheck
)

echo.
echo %GREEN%[INFO]%NC% RabbitMQ 启动成功!
echo 连接信息:
echo   AMQP URL: amqp://%RABBITMQ_USER%:%RABBITMQ_PASSWORD%@localhost:%RABBITMQ_PORT%%RABBITMQ_VHOST%
echo   Management UI: http://localhost:%RABBITMQ_MGMT_PORT%
echo   Username: %RABBITMQ_USER%
echo   Password: %RABBITMQ_PASSWORD%
echo   Virtual Host: %RABBITMQ_VHOST%
echo.
echo %GREEN%[INFO]%NC% 已启用延迟队列插件: rabbitmq_delayed_message_exchange
exit /b 0

:stop
echo %GREEN%[INFO]%NC% 停止 RabbitMQ 容器...
docker stop %CONTAINER_NAME%
echo %GREEN%[INFO]%NC% RabbitMQ 容器已停止
exit /b 0

:restart
call :stop
timeout /t 2 /nobreak >nul
call :start
exit /b 0

:status
echo %GREEN%[INFO]%NC% RabbitMQ 容器状态:
docker ps --filter "name=%CONTAINER_NAME%" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

echo.
echo %GREEN%[INFO]%NC% RabbitMQ 集群状态:
docker exec %CONTAINER_NAME% rabbitmqctl cluster_status 2>nul | more +0

echo.
echo %GREEN%[INFO]%NC% 已安装的插件:
docker exec %CONTAINER_NAME% rabbitmq-plugins list -e 2>nul | findstr "delayed management"

echo.
echo %GREEN%[INFO]%NC% 队列统计:
docker exec %CONTAINER_NAME% rabbitmqctl list_queues name messages consumers 2>nul

echo.
echo %GREEN%[INFO]%NC% 容器资源使用:
docker stats %CONTAINER_NAME% --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}"
exit /b 0

:logs
docker logs -f --tail 100 %CONTAINER_NAME%
exit /b 0

:remove
echo %YELLOW%[WARN]%NC% 这将删除 RabbitMQ 容器 (数据将保留在 %DATA_DIR%)
set /p confirm="确认删除? (y/N): "
if /i "%confirm%"=="y" (
    docker rm -f %CONTAINER_NAME%
    echo %GREEN%[INFO]%NC% RabbitMQ 容器已删除
)
exit /b 0

:create_delay_queue
echo %GREEN%[INFO]%NC% 创建延迟队列示例...

REM 下载 rabbitmqadmin 并创建延迟交换机和队列
docker exec %CONTAINER_NAME% bash -c "curl -s -o /tmp/rabbitmqadmin http://localhost:15672/cli/rabbitmqadmin && chmod +x /tmp/rabbitmqadmin && /tmp/rabbitmqadmin -u %RABBITMQ_USER% -p %RABBITMQ_PASSWORD% -V %RABBITMQ_VHOST% declare exchange name=delay.exchange type=x-delayed-message durable=true arguments='{\"x-delayed-type\":\"direct\"}' && /tmp/rabbitmqadmin -u %RABBITMQ_USER% -p %RABBITMQ_PASSWORD% -V %RABBITMQ_VHOST% declare queue name=delay.queue durable=true && /tmp/rabbitmqadmin -u %RABBITMQ_USER% -p %RABBITMQ_PASSWORD% -V %RABBITMQ_VHOST% declare binding source=delay.exchange destination=delay.queue routing_key=delay.key"

echo %GREEN%[INFO]%NC% 延迟队列创建成功!
echo   Exchange: delay.exchange (type: x-delayed-message)
echo   Queue: delay.queue
echo   Routing Key: delay.key
exit /b 0

:create_queue
echo %GREEN%[INFO]%NC% 创建普通队列示例...

docker exec %CONTAINER_NAME% bash -c "curl -s -o /tmp/rabbitmqadmin http://localhost:15672/cli/rabbitmqadmin && chmod +x /tmp/rabbitmqadmin && /tmp/rabbitmqadmin -u %RABBITMQ_USER% -p %RABBITMQ_PASSWORD% -V %RABBITMQ_VHOST% declare exchange name=task.exchange type=direct durable=true && /tmp/rabbitmqadmin -u %RABBITMQ_USER% -p %RABBITMQ_PASSWORD% -V %RABBITMQ_VHOST% declare queue name=task.queue durable=true && /tmp/rabbitmqadmin -u %RABBITMQ_USER% -p %RABBITMQ_PASSWORD% -V %RABBITMQ_VHOST% declare binding source=task.exchange destination=task.queue routing_key=task.key"

echo %GREEN%[INFO]%NC% 普通队列创建成功!
exit /b 0

:list
echo %GREEN%[INFO]%NC% 所有队列和交换机:
docker exec %CONTAINER_NAME% rabbitmqctl list_queues name messages consumers 2>nul
echo.
docker exec %CONTAINER_NAME% rabbitmqctl list_exchanges name type 2>nul
exit /b 0

:usage
echo 用法: %~nx0 {start^|stop^|restart^|status^|logs^|remove^|create-delay-queue^|create-queue^|list}
echo.
echo 命令说明:
echo   start              - 启动 RabbitMQ
echo   stop               - 停止 RabbitMQ
echo   restart            - 重启 RabbitMQ
echo   status             - 查看状态
echo   logs               - 查看日志
echo   remove             - 删除容器
echo   create-delay-queue - 创建延迟队列示例
echo   create-queue       - 创建普通队列示例
echo   list               - 列出所有队列
echo.
echo 延迟队列使用示例 (Go):
echo   发布消息时设置 headers['x-delay'] = 5000 (毫秒)
exit /b 1
