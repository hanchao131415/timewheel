#!/bin/bash

# MySQL 部署脚本 for TimeWheel
# 用法: ./deploy-mysql.sh [start|stop|restart|status|logs]

set -e

# 配置变量
CONTAINER_NAME="timewheel-mysql"
MYSQL_ROOT_PASSWORD="timewheel123"
MYSQL_DATABASE="timewheel"
MYSQL_USER="timewheel"
MYSQL_PASSWORD="timewheel123"
MYSQL_PORT="3306"
DATA_DIR="./data/mysql"
MYSQL_VERSION="8.0"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 检查 Docker 是否安装
check_docker() {
    if ! command -v docker &> /dev/null; then
        log_error "Docker 未安装，请先安装 Docker"
        exit 1
    fi
}

# 创建数据目录
create_data_dir() {
    if [ ! -d "$DATA_DIR" ]; then
        mkdir -p "$DATA_DIR"
        log_info "创建数据目录: $DATA_DIR"
    fi
}

# 启动 MySQL
start_mysql() {
    check_docker

    # 检查容器是否已存在
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
            log_warn "MySQL 容器已在运行中"
            return 0
        else
            log_info "启动已存在的 MySQL 容器..."
            docker start $CONTAINER_NAME
            return 0
        fi
    fi

    create_data_dir

    log_info "启动 MySQL 容器..."
    docker run -d \
        --name $CONTAINER_NAME \
        --restart=unless-stopped \
        -p $MYSQL_PORT:3306 \
        -e MYSQL_ROOT_PASSWORD=$MYSQL_ROOT_PASSWORD \
        -e MYSQL_DATABASE=$MYSQL_DATABASE \
        -e MYSQL_USER=$MYSQL_USER \
        -e MYSQL_PASSWORD=$MYSQL_PASSWORD \
        -v $(pwd)/$DATA_DIR:/var/lib/mysql \
        -v $(pwd)/../migrations:/docker-entrypoint-initdb.d:ro \
        --character-set-server=utf8mb4 \
        --collation-server=utf8mb4_unicode_ci \
        mysql:$MYSQL_VERSION \
        --default-authentication-plugin=mysql_native_password \
        --max_connections=1000 \
        --innodb_buffer_pool_size=256M \
        --innodb_log_file_size=64M

    log_info "等待 MySQL 启动..."
    sleep 10

    # 健康检查
    for i in {1..30}; do
        if docker exec $CONTAINER_NAME mysqladmin ping -h localhost --silent; then
            log_info "MySQL 启动成功!"
            log_info "连接信息:"
            echo "  Host: localhost"
            echo "  Port: $MYSQL_PORT"
            echo "  Database: $MYSQL_DATABASE"
            echo "  User: $MYSQL_USER"
            echo "  Password: $MYSQL_PASSWORD"
            echo "  Root Password: $MYSQL_ROOT_PASSWORD"
            return 0
        fi
        sleep 1
    done

    log_error "MySQL 启动超时"
    return 1
}

# 停止 MySQL
stop_mysql() {
    check_docker

    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        log_info "停止 MySQL 容器..."
        docker stop $CONTAINER_NAME
        log_info "MySQL 容器已停止"
    else
        log_warn "MySQL 容器未在运行"
    fi
}

# 重启 MySQL
restart_mysql() {
    stop_mysql
    sleep 2
    start_mysql
}

# 查看 MySQL 状态
status_mysql() {
    check_docker

    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        log_info "MySQL 容器状态:"
        docker ps --filter "name=$CONTAINER_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

        echo ""
        log_info "容器资源使用:"
        docker stats $CONTAINER_NAME --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}"
    else
        log_warn "MySQL 容器未在运行"
    fi
}

# 查看 MySQL 日志
logs_mysql() {
    check_docker
    docker logs -f --tail 100 $CONTAINER_NAME
}

# 删除 MySQL 容器
remove_mysql() {
    check_docker

    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        log_warn "这将删除 MySQL 容器 (数据将保留在 $DATA_DIR)"
        read -p "确认删除? (y/N): " confirm
        if [ "$confirm" = "y" ] || [ "$confirm" = "Y" ]; then
            docker rm -f $CONTAINER_NAME
            log_info "MySQL 容器已删除"
        fi
    else
        log_warn "MySQL 容器不存在"
    fi
}

# 进入 MySQL 命令行
cli_mysql() {
    check_docker
    docker exec -it $CONTAINER_NAME mysql -u$MYSQL_USER -p$MYSQL_PASSWORD $MYSQL_DATABASE
}

# 备份数据库
backup_mysql() {
    check_docker
    BACKUP_FILE="timewheel_backup_$(date +%Y%m%d_%H%M%S).sql"
    log_info "备份数据库到: $BACKUP_FILE"
    docker exec $CONTAINER_NAME mysqldump -u$MYSQL_USER -p$MYSQL_PASSWORD $MYSQL_DATABASE > $BACKUP_FILE
    log_info "备份完成: $BACKUP_FILE"
}

# 主命令
case "$1" in
    start)
        start_mysql
        ;;
    stop)
        stop_mysql
        ;;
    restart)
        restart_mysql
        ;;
    status)
        status_mysql
        ;;
    logs)
        logs_mysql
        ;;
    remove)
        remove_mysql
        ;;
    cli)
        cli_mysql
        ;;
    backup)
        backup_mysql
        ;;
    *)
        echo "用法: $0 {start|stop|restart|status|logs|remove|cli|backup}"
        echo ""
        echo "命令说明:"
        echo "  start   - 启动 MySQL"
        echo "  stop    - 停止 MySQL"
        echo "  restart - 重启 MySQL"
        echo "  status  - 查看状态"
        echo "  logs    - 查看日志"
        echo "  remove  - 删除容器"
        echo "  cli     - 进入 MySQL 命令行"
        echo "  backup  - 备份数据库"
        exit 1
        ;;
esac
