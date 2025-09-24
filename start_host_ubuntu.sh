#!/usr/bin/env bash

set -Eeuo pipefail

# 用法:
#   ./start_host_ubuntu.sh [50]
#   - 传入参数 50 时, 固定叠加 docker-compose-5090.yml
#
# 环境变量(可选):
#   APP_PORT             默认为 8090
#   DIGITAL_PEOPLE_DIR   默认为 /mnt/windows-digitalpeople
#   APP_WORKDIR          默认为 $DIGITAL_PEOPLE_DIR/workdir
#   HOST_VOICE_DIR       默认为 $DIGITAL_PEOPLE_DIR/voice/data
#   HOST_VIDEO_DIR       默认为 $DIGITAL_PEOPLE_DIR/face2face
#   HOST_RESULT_DIR      默认为 $DIGITAL_PEOPLE_DIR/face2face/result
#   WIN_COMPANY_DIR      默认为 $DIGITAL_PEOPLE_DIR
#   TTS_BASE_URL         如设置则后端使用该地址, 未设置则沿用代码内默认域名
#   VIDEO_BASE_URL       如设置则后端使用该地址, 未设置则沿用代码内默认域名
#   RABBITMQ_URL         队列地址, 不设置则使用代码默认
#   REDIS_ADDR           Redis 地址, 不设置则使用代码默认

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

GPU_PROFILE=""
if [[ "${1:-}" == "50" ]]; then
  GPU_PROFILE="50"
fi

echo "🚀 在主机(Ubuntu)启动 Go Web 服务, 然后启动 docker compose..."

# 目录变量: 与后端默认保持一致, 可通过环境变量覆盖
export DIGITAL_PEOPLE_DIR="${DIGITAL_PEOPLE_DIR:-/mnt/windows-digitalpeople}"
export APP_WORKDIR="${APP_WORKDIR:-$DIGITAL_PEOPLE_DIR/workdir}"
export HOST_VOICE_DIR="${HOST_VOICE_DIR:-$DIGITAL_PEOPLE_DIR/voice/data}"
export HOST_VIDEO_DIR="${HOST_VIDEO_DIR:-$DIGITAL_PEOPLE_DIR/face2face}"
export HOST_RESULT_DIR="${HOST_RESULT_DIR:-$DIGITAL_PEOPLE_DIR/face2face/result}"
export WIN_COMPANY_DIR="${WIN_COMPANY_DIR:-$DIGITAL_PEOPLE_DIR}"
export APP_PORT="${APP_PORT:-8090}"

mkdir -p "$DIGITAL_PEOPLE_DIR" "$APP_WORKDIR" "$HOST_VOICE_DIR" "$HOST_VIDEO_DIR" "$HOST_RESULT_DIR"

# 选择 docker compose 命令
if command -v docker &>/dev/null && docker compose version &>/dev/null; then
  DC_BASE=(docker compose)
else
  DC_BASE=(docker-compose)
fi

COMPOSE_FILES=("-f" "docker-compose-linux.yml")
if [[ -n "$GPU_PROFILE" ]]; then
  if [[ -f "$ROOT_DIR/docker-compose-5090.yml" ]]; then
    COMPOSE_FILES+=("-f" "docker-compose-5090.yml")
    echo "🧩 使用 docker-compose-5090.yml 叠加"
  else
    echo "⚠️ 未找到 docker-compose-5090.yml, 仅使用 docker-compose-linux.yml"
  fi
fi

# 启动 Go Web (后台)
echo "🌐 启动 Go Web (端口 :$APP_PORT) ..."
(
  cd "$ROOT_DIR/server"
  # 如需将日志输出到文件, 可改为: nohup go run . >> server.log 2>&1 &
  nohup go run . >> server.log 2>&1 &
  echo $! > heygem_web.pid
)
sleep 1
echo "✅ Go Web 已启动, PID: $(cat "$ROOT_DIR/server/heygem_web.pid" 2>/dev/null || echo -n '?')"

# 启动 docker compose
echo "🐳 启动 Docker Compose 服务..."
(
  cd "$ROOT_DIR"
  "${DC_BASE[@]}" "${COMPOSE_FILES[@]}" up -d
)

echo "✅ 所有服务已启动"
echo "- Web 界面:   http://<你的主机IP>:$APP_PORT"
echo "- 可选依赖:   TTS_BASE_URL=${TTS_BASE_URL:-'(未设置, 使用默认域名)'}  VIDEO_BASE_URL=${VIDEO_BASE_URL:-'(未设置, 使用默认域名)'}"


