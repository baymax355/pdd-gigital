#!/bin/bash

echo "🚀 启动胖哒哒数字人系统..."

# 构建前端
echo "📦 构建前端..."
cd client
npm run build
cd ..

# 启动所有服务
echo "🐳 启动Docker服务..."
docker-compose -f docker-compose-linux.yml up -d

echo "✅ 系统启动完成！"
echo "🌐 Web界面: http://localhost:8090"
echo "📊 TTS服务: http://localhost:18180"
echo "🎬 视频服务: http://localhost:8383"

# 显示服务状态
echo ""
echo "📋 服务状态:"
docker-compose -f docker-compose-linux.yml ps