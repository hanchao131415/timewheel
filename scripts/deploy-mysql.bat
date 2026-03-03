@echo off
REM MySQL 部署脚本 for TimeWheel (Windows)
REM 用法: deploy-mysql.bat [start|stop|restart|status|logs]

setlocal enabledelayedexpansion

REM 配置变量
set CONTAINER_NAME=timewheel-mysql
set MYSQL_ROOT_PASSWORD=timewheel123
set MYSQL_DATABASE=timewheel
set MYSQL_USER=timewheel
set MYSQL_PASSWORD=timewheel123
set MYSQL_PORT=3306
set DATA_DIR=.\data\mysql
set MYSQL_VERSION=8.0

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
goto usage

:start
echo %GREEN%[INFO]%NC% 启动 MySQL...

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
        echo %YELLOW%[WARN]%NC% MySQL 容器已在运行中
        exit /b 0
    )
    echo %GREEN%[INFO]%NC% 启动已存在的 MySQL 容器...
    docker start %CONTAINER_NAME%
    exit /b 0
)

REM 创建数据目录
if not exist "%DATA_DIR%" mkdir "%DATA_DIR%"

REM 启动容器
docker run -d ^
    --name %CONTAINER_NAME% ^
    --restart=unless-stopped ^
    -p %MYSQL_PORT%:3306 ^
    -e MYSQL_ROOT_PASSWORD=%MYSQL_ROOT_PASSWORD% ^
    -e MYSQL_DATABASE=%MYSQL_DATABASE% ^
    -e MYSQL_USER=%MYSQL_USER% ^
    -e MYSQL_PASSWORD=%MYSQL_PASSWORD% ^
    -v %cd%\%DATA_DIR%:/var/lib/mysql ^
    mysql:%MYSQL_VERSION% ^
    --default-authentication-plugin=mysql_native_password ^
    --max_connections=1000

echo %GREEN%[INFO]%NC% 等待 MySQL 启动...
timeout /t 15 /nobreak >nul

echo.
echo %GREEN%[INFO]%NC% MySQL 启动成功!
echo 连接信息:
echo   Host: localhost
echo   Port: %MYSQL_PORT%
echo   Database: %MYSQL_DATABASE%
echo   User: %MYSQL_USER%
echo   Password: %MYSQL_PASSWORD%
echo   Root Password: %MYSQL_ROOT_PASSWORD%
exit /b 0

:stop
echo %GREEN%[INFO]%NC% 停止 MySQL 容器...
docker stop %CONTAINER_NAME%
echo %GREEN%[INFO]%NC% MySQL 容器已停止
exit /b 0

:restart
call :stop
timeout /t 2 /nobreak >nul
call :start
exit /b 0

:status
echo %GREEN%[INFO]%NC% MySQL 容器状态:
docker ps --filter "name=%CONTAINER_NAME%" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
echo.
echo %GREEN%[INFO]%NC% 容器资源使用:
docker stats %CONTAINER_NAME% --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}"
exit /b 0

:logs
docker logs -f --tail 100 %CONTAINER_NAME%
exit /b 0

:remove
echo %YELLOW%[WARN]%NC% 这将删除 MySQL 容器 (数据将保留在 %DATA_DIR%)
set /p confirm="确认删除? (y/N): "
if /i "%confirm%"=="y" (
    docker rm -f %CONTAINER_NAME%
    echo %GREEN%[INFO]%NC% MySQL 容器已删除
)
exit /b 0

:cli
docker exec -it %CONTAINER_NAME% mysql -u%MYSQL_USER% -p%MYSQL_PASSWORD% %MYSQL_DATABASE%
exit /b 0

:usage
echo 用法: %~nx0 {start^|stop^|restart^|status^|logs^|remove^|cli}
echo.
echo 命令说明:
echo   start   - 启动 MySQL
echo   stop    - 停止 MySQL
echo   restart - 重启 MySQL
echo   status  - 查看状态
echo   logs    - 查看日志
echo   remove  - 删除容器
echo   cli     - 进入 MySQL 命令行
exit /b 1
