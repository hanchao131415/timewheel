@echo off
REM Redis 部署脚本 for TimeWheel (Windows)
REM 用法: deploy-redis.bat [start|stop|restart|status|logs]

setlocal enabledelayedexpansion

REM 配置变量
set CONTAINER_NAME=timewheel-redis
set REDIS_PORT=6379
set REDIS_PASSWORD=timewheel123
set DATA_DIR=.\data\redis
set REDIS_VERSION=7-alpine

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
if "%1"=="cli" goto cli
if "%1"=="monitor" goto monitor
if "%1"=="stats" goto stats
goto usage

:start
echo %GREEN%[INFO]%NC% 启动 Redis...

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
        echo %YELLOW%[WARN]%NC% Redis 容器已在运行中
        exit /b 0
    )
    echo %GREEN%[INFO]%NC% 启动已存在的 Redis 容器...
    docker start %CONTAINER_NAME%
    exit /b 0
)

REM 创建数据目录
if not exist "%DATA_DIR%" mkdir "%DATA_DIR%"

REM 启动容器
docker run -d ^
    --name %CONTAINER_NAME% ^
    --restart=unless-stopped ^
    -p %REDIS_PORT%:6379 ^
    -v %cd%\%DATA_DIR%:/data ^
    redis:%REDIS_VERSION% ^
    redis-server ^
    --requirepass %REDIS_PASSWORD% ^
    --appendonly yes ^
    --maxmemory 256mb ^
    --maxmemory-policy allkeys-lru ^
    --save 60 1000

echo %GREEN%[INFO]%NC% 等待 Redis 启动...
timeout /t 5 /nobreak >nul

echo.
echo %GREEN%[INFO]%NC% Redis 启动成功!
echo 连接信息:
echo   Host: localhost
echo   Port: %REDIS_PORT%
echo   Password: %REDIS_PASSWORD%
echo.
echo 连接命令: redis-cli -h localhost -p %REDIS_PORT% -a %REDIS_PASSWORD%
exit /b 0

:stop
echo %GREEN%[INFO]%NC% 停止 Redis 容器...
docker stop %CONTAINER_NAME%
echo %GREEN%[INFO]%NC% Redis 容器已停止
exit /b 0

:restart
call :stop
timeout /t 2 /nobreak >nul
call :start
exit /b 0

:status
echo %GREEN%[INFO]%NC% Redis 容器状态:
docker ps --filter "name=%CONTAINER_NAME%" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
echo.
echo %GREEN%[INFO]%NC% 容器资源使用:
docker stats %CONTAINER_NAME% --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}"
exit /b 0

:logs
docker logs -f --tail 100 %CONTAINER_NAME%
exit /b 0

:remove
echo %YELLOW%[WARN]%NC% 这将删除 Redis 容器 (数据将保留在 %DATA_DIR%)
set /p confirm="确认删除? (y/N): "
if /i "%confirm%"=="y" (
    docker rm -f %CONTAINER_NAME%
    echo %GREEN%[INFO]%NC% Redis 容器已删除
)
exit /b 0

:cli
docker exec -it %CONTAINER_NAME% redis-cli -a %REDIS_PASSWORD%
exit /b 0

:monitor
echo %GREEN%[INFO]%NC% Redis 实时监控 (Ctrl+C 退出):
docker exec -it %CONTAINER_NAME% redis-cli -a %REDIS_PASSWORD% MONITOR
exit /b 0

:stats
echo %GREEN%[INFO]%NC% Redis 统计信息:
echo.
echo === 内存使用 ===
docker exec %CONTAINER_NAME% redis-cli -a %REDIS_PASSWORD% INFO memory 2>nul | findstr "used_memory_human used_memory_peak_human maxmemory_human"
echo.
echo === 键统计 ===
docker exec %CONTAINER_NAME% redis-cli -a %REDIS_PASSWORD% DBSIZE 2>nul
echo.
echo === 命令统计 ===
docker exec %CONTAINER_NAME% redis-cli -a %REDIS_PASSWORD% INFO stats 2>nul | findstr "total_connections_received total_commands_processed instantaneous_ops_per_sec"
exit /b 0

:usage
echo 用法: %~nx0 {start^|stop^|restart^|status^|logs^|remove^|cli^|monitor^|stats}
echo.
echo 命令说明:
echo   start   - 启动 Redis
echo   stop    - 停止 Redis
echo   restart - 重启 Redis
echo   status  - 查看状态
echo   logs    - 查看日志
echo   remove  - 删除容器
echo   cli     - 进入 Redis 命令行
echo   monitor - 实时监控
echo   stats   - 查看统计
exit /b 1
