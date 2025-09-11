#!/bin/bash

echo "🛑 停止胖哒哒数字人系统..."

# 停止所有服务
docker-compose -f docker-compose-linux.yml down

echo "✅ 系统已停止！"