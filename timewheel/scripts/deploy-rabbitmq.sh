#!/bin/bash

# RabbitMQ 部署脚本 for TimeWheel (支持延迟队列)
# 用法: ./deploy-rabbitmq.sh [start|stop|restart|status|logs]

set -e

# 配置变量
CONTAINER_NAME="timewheel-rabbitmq"
RABBITMQ_PORT="5672"           # AMQP 协议端口
RABBITMQ_MGMT_PORT="15672"     # 管理界面端口
RABBITMQ_USER="timewheel"
RABBITMQ_PASSWORD="timewheel123"
RABBITMQ_VHOST="/timewheel"
DATA_DIR="./data/rabbitmq"
RABBITMQ_VERSION="3.13-management"

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

# 启动 RabbitMQ
start_rabbitmq() {
    check_docker

    # 检查容器是否已存在
    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
            log_warn "RabbitMQ 容器已在运行中"
            return 0
        else
            log_info "启动已存在的 RabbitMQ 容器..."
            docker start $CONTAINER_NAME
            return 0
        fi
    fi

    create_data_dir

    log_info "启动 RabbitMQ 容器 (带延迟队列插件)..."
    docker run -d \
        --name $CONTAINER_NAME \
        --restart=unless-stopped \
        -p $RABBITMQ_PORT:5672 \
        -p $RABBITMQ_MGMT_PORT:15672 \
        -e RABBITMQ_DEFAULT_USER=$RABBITMQ_USER \
        -e RABBITMQ_DEFAULT_PASS=$RABBITMQ_PASSWORD \
        -e RABBITMQ_DEFAULT_VHOST=$RABBITMQ_VHOST \
        -v $(pwd)/$DATA_DIR:/var/lib/rabbitmq \
        rabbitmq:$RABBITMQ_VERSION

    log_info "等待 RabbitMQ 启动..."
    sleep 15

    # 安装延迟队列插件
    log_info "安装延迟队列插件..."
    docker exec $CONTAINER_NAME rabbitmq-plugins enable rabbitmq_delayed_message_exchange

    # 重启以加载插件
    docker restart $CONTAINER_NAME
    sleep 10

    # 健康检查
    for i in {1..30}; do
        if docker exec $CONTAINER_NAME rabbitmqctl status &> /dev/null; then
            log_info "RabbitMQ 启动成功!"
            log_info "连接信息:"
            echo "  AMQP URL: amqp://$RABBITMQ_USER:$RABBITMQ_PASSWORD@localhost:$RABBITMQ_PORT$RABBITMQ_VHOST"
            echo "  Management UI: http://localhost:$RABBITMQ_MGMT_PORT"
            echo "  Username: $RABBITMQ_USER"
            echo "  Password: $RABBITMQ_PASSWORD"
            echo "  Virtual Host: $RABBITMQ_VHOST"
            echo ""
            log_info "已启用延迟队列插件: rabbitmq_delayed_message_exchange"
            return 0
        fi
        sleep 1
    done

    log_error "RabbitMQ 启动超时"
    return 1
}

# 停止 RabbitMQ
stop_rabbitmq() {
    check_docker

    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        log_info "停止 RabbitMQ 容器..."
        docker stop $CONTAINER_NAME
        log_info "RabbitMQ 容器已停止"
    else
        log_warn "RabbitMQ 容器未在运行"
    fi
}

# 重启 RabbitMQ
restart_rabbitmq() {
    stop_rabbitmq
    sleep 2
    start_rabbitmq
}

# 查看 RabbitMQ 状态
status_rabbitmq() {
    check_docker

    if docker ps --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        log_info "RabbitMQ 容器状态:"
        docker ps --filter "name=$CONTAINER_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"

        echo ""
        log_info "RabbitMQ 集群状态:"
        docker exec $CONTAINER_NAME rabbitmqctl cluster_status 2>/dev/null | head -20

        echo ""
        log_info "已安装的插件:"
        docker exec $CONTAINER_NAME rabbitmq-plugins list -e 2>/dev/null | grep -E "delayed|management"

        echo ""
        log_info "队列统计:"
        docker exec $CONTAINER_NAME rabbitmqctl list_queues name messages consumers 2>/dev/null

        echo ""
        log_info "容器资源使用:"
        docker stats $CONTAINER_NAME --no-stream --format "table {{.Container}}\t{{.CPUPerc}}\t{{.MemUsage}}"
    else
        log_warn "RabbitMQ 容器未在运行"
    fi
}

# 查看 RabbitMQ 日志
logs_rabbitmq() {
    check_docker
    docker logs -f --tail 100 $CONTAINER_NAME
}

# 删除 RabbitMQ 容器
remove_rabbitmq() {
    check_docker

    if docker ps -a --format '{{.Names}}' | grep -q "^${CONTAINER_NAME}$"; then
        log_warn "这将删除 RabbitMQ 容器 (数据将保留在 $DATA_DIR)"
        read -p "确认删除? (y/N): " confirm
        if [ "$confirm" = "y" ] || [ "$confirm" = "Y" ]; then
            docker rm -f $CONTAINER_NAME
            log_info "RabbitMQ 容器已删除"
        fi
    else
        log_warn "RabbitMQ 容器不存在"
    fi
}

# 创建延迟队列示例
create_delay_queue() {
    check_docker

    log_info "创建延迟队列示例..."

    # 使用 rabbitmqadmin 创建延迟交换机和队列
    docker exec $CONTAINER_NAME bash -c "
        # 下载 rabbitmqadmin
        curl -s -o /tmp/rabbitmqadmin http://localhost:15672/cli/rabbitmqadmin
        chmod +x /tmp/rabbitmqadmin

        # 创建延迟交换机
        /tmp/rabbitmqadmin -u $RABBITMQ_USER -p $RABBITMQ_PASSWORD -V $RABBITMQ_VHOST \
            declare exchange name=delay.exchange type=x-delayed-message \
            durable=true arguments='{\"x-delayed-type\":\"direct\"}'

        # 创建延迟队列
        /tmp/rabbitmqadmin -u $RABBITMQ_USER -p $RABBITMQ_PASSWORD -V $RABBITMQ_VHOST \
            declare queue name=delay.queue durable=true

        # 绑定队列到交换机
        /tmp/rabbitmqadmin -u $RABBITMQ_USER -p $RABBITMQ_PASSWORD -V $RABBITMQ_VHOST \
            declare binding source=delay.exchange destination=delay.queue routing_key=delay.key
    "

    log_info "延迟队列创建成功!"
    echo "  Exchange: delay.exchange (type: x-delayed-message)"
    echo "  Queue: delay.queue"
    echo "  Routing Key: delay.key"
}

# 创建普通队列
create_normal_queue() {
    check_docker

    log_info "创建普通队列示例..."

    docker exec $CONTAINER_NAME bash -c "
        curl -s -o /tmp/rabbitmqadmin http://localhost:15672/cli/rabbitmqadmin
        chmod +x /tmp/rabbitmqadmin

        # 创建直连交换机
        /tmp/rabbitmqadmin -u $RABBITMQ_USER -p $RABBITMQ_PASSWORD -V $RABBITMQ_VHOST \
            declare exchange name=task.exchange type=direct durable=true

        # 创建队列
        /tmp/rabbitmqadmin -u $RABBITMQ_USER -p $RABBITMQ_PASSWORD -V $RABBITMQ_VHOST \
            declare queue name=task.queue durable=true

        # 绑定
        /tmp/rabbitmqadmin -u $RABBITMQ_USER -p $RABBITMQ_PASSWORD -V $RABBITMQ_VHOST \
            declare binding source=task.exchange destination=task.queue routing_key=task.key
    "

    log_info "普通队列创建成功!"
}

# 列出所有队列
list_queues() {
    check_docker
    log_info "所有队列和交换机:"
    docker exec $CONTAINER_NAME rabbitmqctl list_queues name messages consumers 2>/dev/null
    echo ""
    docker exec $CONTAINER_NAME rabbitmqctl list_exchanges name type 2>/dev/null | grep -v "^listing"
}

# 主命令
case "$1" in
    start)
        start_rabbitmq
        ;;
    stop)
        stop_rabbitmq
        ;;
    restart)
        restart_rabbitmq
        ;;
    status)
        status_rabbitmq
        ;;
    logs)
        logs_rabbitmq
        ;;
    remove)
        remove_rabbitmq
        ;;
    create-delay-queue)
        create_delay_queue
        ;;
    create-queue)
        create_normal_queue
        ;;
    list)
        list_queues
        ;;
    *)
        echo "用法: $0 {start|stop|restart|status|logs|remove|create-delay-queue|create-queue|list}"
        echo ""
        echo "命令说明:"
        echo "  start              - 启动 RabbitMQ"
        echo "  stop               - 停止 RabbitMQ"
        echo "  restart            - 重启 RabbitMQ"
        echo "  status             - 查看状态"
        echo "  logs               - 查看日志"
        echo "  remove             - 删除容器"
        echo "  create-delay-queue - 创建延迟队列示例"
        echo "  create-queue       - 创建普通队列示例"
        echo "  list               - 列出所有队列"
        echo ""
        echo "延迟队列使用示例 (Go):"
        echo "  发布消息时设置 headers['x-delay'] = 5000 (毫秒)"
        exit 1
        ;;
esac
