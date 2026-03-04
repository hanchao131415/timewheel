#!/bin/bash

# TimeWheel 停止所有服务脚本 (Linux/Mac)

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
echo "   停止 TimeWheel 基础设施服务"
echo "=========================================="
echo

log_info "停止所有服务..."

# 切换到项目根目录
cd "$(dirname "$0")/.."

docker-compose -f deployments/docker-compose.infra.yaml down

echo
log_info "所有服务已停止"
echo
echo "如需删除数据卷，请运行:"
echo "  docker-compose -f deployments/docker-compose.infra.yaml down -v"
echo
