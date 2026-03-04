#!/bin/bash

# Redis 部署脚本 for TimeWheel
# 用法: ./deploy-redis.sh [start|stop|restart|status|logs]

set -e

# 配置变量
CONTAINER_NAME="timewheel-redis"
REDIS_PORT="6379"
REDIS_PASSWORD="timewheel123"
DATA_DIR="./data/redis"
REDIS_VERSION="7-alpine"

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

# 启动 Redis
start_redis() {
    check_docker

    # 检查容器是否已存在
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
            log_warn "Redis 容器已在运行中"
            return 0
        else
            log_info "启动已存在的 Redis 容器..."
            docker start $CONTAINER_NAME
            return 0
        fi
    fi

    create_data_dir

    log_info "启动 Redis 容器..."
    docker run -d \
        --name $CONTAINER_NAME \
        --restart=unless-stopped \
        -p $REDIS_PORT:6379 \
        -v $(pwd)/$DATA_DIR:/data \
        redis:$REDIS_VERSION \
        redis-server \
        --requirepass $REDIS_PASSWORD \
        --appendonly yes \
        --maxmemory 256mb \
        --maxmemory-policy allkeys-lru \
        --save 60 1000

    log_info "等待 Redis 启动..."
    sleep 3

    # 健康检查
    for i in {1..30}; do
        if docker exec $CONTAINER_NAME redis-cli -a $REDIS_PASSWORD ping 2>/dev/null | grep -q "PONG"; then
            log_info "Redis 启动成功!"
            log_info "连接信息:"
            echo "  Host: localhost"
            echo "  Port: $REDIS_PORT"
            echo "  Password: $REDIS_PASSWORD"
            echo ""
            echo "连接命令: redis-cli -h localhost -p $REDIS_PORT -a $REDIS_PASSWORD"
            return 0
        fi
        sleep 1
    done

    log_error "Redis 启动超时"
    return 1
}

# 停止 Redis
stop_redis() {
    check_docker

    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        log_info "停止 Redis 容器..."
        docker stop $CONTAINER_NAME
        log_info "Redis 容器已停止"
    else
        log_warn "Redis 容器未在运行"
    fi
}

# 重启 Redis
restart_redis() {
    stop_redis
    sleep 2
    start_redis
}

# 查看 Redis 状态
status_redis() {
    check_docker

    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        log_info "Redis 容器状态:"
        docker ps --filter "name=$CONTAINER_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

        echo ""
        log_info "Redis 信息:"
        docker exec $CONTAINER_NAME redis-cli -a $REDIS_PASSWORD INFO server 2>/dev/null | grep -E "redis_version|uptime_in_seconds|connected_clients|used_memory_human"

        echo ""
        log_info "容器资源使用:"
        docker stats $CONTAINER_NAME --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}"
    else
        log_warn "Redis 容器未在运行"
    fi
}

# 查看 Redis 日志
logs_redis() {
    check_docker
    docker logs -f --tail 100 $CONTAINER_NAME
}

# 删除 Redis 容器
remove_redis() {
    check_docker

    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        log_warn "这将删除 Redis 容器 (数据将保留在 $DATA_DIR)"
        read -p "确认删除? (y/N): " confirm
        if [ "$confirm" = "y" ] || [ "$confirm" = "Y" ]; then
            docker rm -f $CONTAINER_NAME
            log_info "Redis 容器已删除"
        fi
    else
        log_warn "Redis 容器不存在"
    fi
}

# 进入 Redis 命令行
cli_redis() {
    check_docker
    docker exec -it $CONTAINER_NAME redis-cli -a $REDIS_PASSWORD
}

# 监控 Redis
monitor_redis() {
    check_docker
    log_info "Redis 实时监控 (Ctrl+C 退出):"
    docker exec -it $CONTAINER_NAME redis-cli -a $REDIS_PASSWORD MONITOR
}

# 查看 Redis 统计
stats_redis() {
    check_docker
    log_info "Redis 统计信息:"
    echo ""
    echo "=== 内存使用 ==="
    docker exec $CONTAINER_NAME redis-cli -a $REDIS_PASSWORD INFO memory 2>/dev/null | grep -E "used_memory_human|used_memory_peak_human|maxmemory_human"

    echo ""
    echo "=== 客户端连接 ==="
    docker exec $CONTAINER_NAME redis-cli -a $REDIS_PASSWORD INFO clients 2>/dev/null

    echo ""
    echo "=== 键统计 ==="
    docker exec $CONTAINER_NAME redis-cli -a $REDIS_PASSWORD DBSIZE 2>/dev/null

    echo ""
    echo "=== 命令统计 ==="
    docker exec $CONTAINER_NAME redis-cli -a $REDIS_PASSWORD INFO stats 2>/dev/null | grep -E "total_connections_received|total_commands_processed|instantaneous_ops_per_sec"
}

# 清理 Redis 数据
flush_redis() {
    check_docker
    log_warn "警告: 这将清空所有 Redis 数据!"
    read -p "确认清空? (yes/N): " confirm
    if [ "$confirm" = "yes" ]; then
        docker exec $CONTAINER_NAME redis-cli -a $REDIS_PASSWORD FLUSHALL 2>/dev/null
        log_info "Redis 数据已清空"
    else
        log_info "操作已取消"
    fi
}

# 主命令
case "$1" in
    start)
        start_redis
        ;;
    stop)
        stop_redis
        ;;
    restart)
        restart_redis
        ;;
    status)
        status_redis
        ;;
    logs)
        logs_redis
        ;;
    remove)
        remove_redis
        ;;
    cli)
        cli_redis
        ;;
    monitor)
        monitor_redis
        ;;
    stats)
        stats_redis
        ;;
    flush)
        flush_redis
        ;;
    *)
        echo "用法: $0 {start|stop|restart|status|logs|remove|cli|monitor|stats|flush}"
        echo ""
        echo "命令说明:"
        echo "  start   - 启动 Redis"
        echo "  stop    - 停止 Redis"
        echo "  restart - 重启 Redis"
        echo "  status  - 查看状态"
        echo "  logs    - 查看日志"
        echo "  remove  - 删除容器"
        echo "  cli     - 进入 Redis 命令行"
        echo "  monitor - 实时监控"
        echo "  stats   - 查看统计"
        echo "  flush   - 清空数据"
        exit 1
        ;;
esac
