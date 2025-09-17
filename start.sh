#!/bin/bash

set -Eeuo pipefail
trap 'echo "❌ 启动失败，请查看上方日志。"; exit 1' ERR

#
# 使用方法:
#   ./start.sh [web|all] [--skip-build] [--rebuild]
#   - web         仅启动后端服务 heygem-web（默认，避免拉取 GPU 依赖）
#   - all         启动全部服务（包含 TTS 与视频，需 GPU 与可用镜像源）
#   - --skip-build  跳过前端构建
#   - --rebuild     先执行镜像重建（web 模式仅重建 heygem-web）
#

MODE="web"
REBUILD=0
SKIP_BUILD=0
GPU_PROFILE="default"

for arg in "$@"; do
  case "$arg" in
    web|all)
      MODE="$arg" ;;
    --skip-build)
      SKIP_BUILD=1 ;;
    --rebuild)
      REBUILD=1 ;;
    50|gpu50|--gpu50|--gpu=50|--gpu-profile=50)
      GPU_PROFILE="50" ;;
    -h|--help)
      echo "用法: ./start.sh [web|all] [--skip-build] [--rebuild] [50]"; exit 0 ;;
    *)
      echo "⚠️ 未知参数: $arg" ;;
  esac
done

echo "🚀 启动胖哒哒数字人系统 (模式: $MODE) ..."

# 确保公司目录挂载存在（与 docker-compose 变量保持一致）
export COMPANY_DIR="${COMPANY_DIR:-./company}"
mkdir -p "$COMPANY_DIR"

# 构建前端
if [[ "$SKIP_BUILD" -eq 0 ]]; then
  echo "📦 构建前端..."
  pushd client >/dev/null
  if [[ ! -d node_modules ]]; then
    echo "📦 安装前端依赖 (优先 npm ci)..."
    (npm ci || npm install)
  fi
  npm run build
  popd >/dev/null
else
  echo "⏭️ 跳过前端构建"
fi

if [[ "$GPU_PROFILE" == "50" ]]; then
  echo "🧩 使用 50 系 GPU 配置 (docker-compose-5090.yml)..."
  DC="docker-compose -f docker-compose-linux.yml -f docker-compose-5090.yml"
else
  DC="docker-compose -f docker-compose-linux.yml"
fi

# 可选重建镜像
if [[ "$REBUILD" -eq 1 ]]; then
  if [[ "$MODE" == "web" ]]; then
    echo "🔧 仅重建 heygem-web 镜像..."
    $DC build --no-cache heygem-web
  else
    echo "🔧 重建全部服务镜像..."
    $DC build --no-cache
  fi
fi

# 启动服务
echo "🐳 启动 Docker 服务..."
if [[ "$MODE" == "web" ]]; then
  $DC up -d --no-deps heygem-web
else
  $DC up -d
fi

echo "✅ 系统启动完成！"
echo "🌐 Web界面: http://localhost:8090"
echo "📊 TTS服务: http://localhost:18180"
echo "🎬 视频服务: http://localhost:8383"

echo ""
echo "📋 服务状态:"
$DC ps
