#!/bin/bash

# TimeWheel 一键部署脚本 (Linux/Mac)
# 启动所有基础设施服务 (MySQL + Redis)

set -e

# 颜色输出
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

echo "=========================================="
echo "   TimeWheel 基础设施一键部署"
echo "=========================================="
echo

# 检查 Docker
log_info "[1/3] 检查 Docker..."
if ! command -v docker &> /dev/null; then
    log_error "Docker 未安装，请先安装 Docker"
    exit 1
fi

if ! command -v docker-compose &> /dev/null && ! docker compose version &> /dev/null; then
    log_error "Docker Compose 未安装"
    exit 1
fi

echo "      Docker 已安装"

# 启动服务
echo
log_info "[2/3] 启动基础设施服务..."

# 切换到项目根目录
cd "$(dirname "$0")/.."

docker-compose -f deployments/docker-compose.infra.yaml up -d

echo
log_info "[3/3] 等待服务就绪..."
sleep 10

# 显示服务状态
echo
echo "=========================================="
echo "   服务状态"
echo "=========================================="
docker-compose -f deployments/docker-compose.infra.yaml ps

echo
echo "=========================================="
echo "   连接信息"
echo "=========================================="
echo
echo "MySQL:"
echo "  Host: localhost"
echo "  Port: 3306"
echo "  Database: timewheel"
echo "  User: timewheel"
echo "  Password: timewheel123"
echo "  Root Password: timewheel123"
echo "  Connection String: timewheel:timewheel123@tcp(localhost:3306)/timewheel?charset=utf8mb4&parseTime=True&loc=Local"
echo
echo "Redis:"
echo "  Host: localhost"
echo "  Port: 6379"
echo "  Password: timewheel123"
echo "  Connection String: redis://:timewheel123@localhost:6379/0"
echo
echo "RabbitMQ:"
echo "  Host: localhost"
echo "  AMQP Port: 5672"
echo "  Management UI: http://localhost:15672"
echo "  Username: timewheel"
echo "  Password: timewheel123"
echo "  Virtual Host: /timewheel"
echo "  Connection URL: amqp://timewheel:timewheel123@localhost:5672/timewheel"
echo
echo "=========================================="
echo "   管理命令"
echo "=========================================="
echo
echo "  停止所有服务:  docker-compose -f deployments/docker-compose.infra.yaml down"
echo "  查看日志:      docker-compose -f deployments/docker-compose.infra.yaml logs -f"
echo "  重启服务:      docker-compose -f deployments/docker-compose.infra.yaml restart"
echo
echo "  MySQL 独立管理: scripts/deploy-mysql.sh [start|stop|status]"
echo "  Redis 独立管理: scripts/deploy-redis.sh [start|stop|status]"
echo "  RabbitMQ 独立管理: scripts/deploy-rabbitmq.sh [start|stop|status]"
echo
echo "延迟队列插件启用 (首次启动后执行):"
echo "  docker exec timewheel-rabbitmq rabbitmq-plugins enable rabbitmq_delayed_message_exchange"
echo
