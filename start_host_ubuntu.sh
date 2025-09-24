#!/usr/bin/env bash

set -Eeuo pipefail

# 用法:
#   ./start_host_ubuntu.sh [50] [ip]
#   - 50  : 固定叠加 docker-compose-5090.yml
#   - ip  : 自动将上游地址设置为 127.0.0.1 (TTS/VIDEO/FUNASR)
#
# 环境变量(可选):
#   APP_PORT             默认为 8090
#   DIGITAL_PEOPLE_DIR   默认为 /mnt/windows-digitalpeople
#   APP_WORKDIR          默认为 $DIGITAL_PEOPLE_DIR/workdir
#   HOST_VOICE_DIR       默认为 $DIGITAL_PEOPLE_DIR/voice/data
#   HOST_VIDEO_DIR       默认为 $DIGITAL_PEOPLE_DIR/face2face
#   HOST_RESULT_DIR      默认为 $DIGITAL_PEOPLE_DIR/face2face/result
#   WIN_COMPANY_DIR      默认为 $DIGITAL_PEOPLE_DIR
#   SKIP_MOUNT           设为 1 则跳过共享盘自动挂载
#   SHARE_PATH           CIFS 源(默认 //192.168.7.10/DIGITALPEOPLE)
#   CIFS_USER            CIFS 用户(默认 administrator)
#   CIFS_PASS            CIFS 密码(默认 Pddold)
#   CIFS_VERSION         CIFS 协议版本(默认 3.0)
#   TTS_BASE_URL         如设置则后端使用该地址, 未设置则沿用代码内默认域名
#   VIDEO_BASE_URL       如设置则后端使用该地址, 未设置则沿用代码内默认域名
#   RABBITMQ_URL         队列地址, 不设置则使用代码默认
#   REDIS_ADDR           Redis 地址, 不设置则使用代码默认

ROOT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)

GPU_PROFILE=""
USE_LOCAL_IP=0

for arg in "$@"; do
  case "$arg" in
    50)
      GPU_PROFILE="50" ;;
    ip|--ip)
      USE_LOCAL_IP=1 ;;
    -h|--help)
      echo "用法: $0 [50] [ip]"; exit 0 ;;
    *) ;;
  esac
done

echo "🚀 在主机(Ubuntu)启动 Go Web 服务, 然后启动 docker compose..."

# 目录变量: 与后端默认保持一致, 可通过环境变量覆盖
export DIGITAL_PEOPLE_DIR="${DIGITAL_PEOPLE_DIR:-/mnt/windows-digitalpeople}"
export APP_WORKDIR="${APP_WORKDIR:-$DIGITAL_PEOPLE_DIR/workdir}"
export HOST_VOICE_DIR="${HOST_VOICE_DIR:-$DIGITAL_PEOPLE_DIR/voice/data}"
export HOST_VIDEO_DIR="${HOST_VIDEO_DIR:-$DIGITAL_PEOPLE_DIR/face2face}"
export HOST_RESULT_DIR="${HOST_RESULT_DIR:-$DIGITAL_PEOPLE_DIR/face2face/result}"
export WIN_COMPANY_DIR="${WIN_COMPANY_DIR:-$DIGITAL_PEOPLE_DIR}"
export APP_PORT="${APP_PORT:-8090}"

# 判定某路径是否已挂载 (mountpoint/findmnt/读取 /proc/mounts 多重回退)
is_mounted() {
  local path="$1"
  if command -v mountpoint >/dev/null 2>&1; then
    mountpoint -q "$path" && return 0
  fi
  if command -v findmnt >/dev/null 2>&1; then
    findmnt -rn "$path" >/dev/null 2>&1 && return 0
  fi
  grep -qs " $(printf '%s' "$path" | sed 's/[[:space:]]/\\040/g') " /proc/mounts && return 0
  return 1
}

# 若指定 ip 参数, 覆盖上游到 127.0.0.1
if [[ "$USE_LOCAL_IP" -eq 1 ]]; then
  export TTS_BASE_URL="http://127.0.0.1:18180"
  export VIDEO_BASE_URL="http://127.0.0.1:8383"
  export FUNASR_BASE_URL="http://127.0.0.1:10095"
  echo "🌐 已根据 'ip' 参数设置 TTS/VIDEO/FUNASR 到 127.0.0.1"
fi

# 尝试共享盘挂载
if [[ "${SKIP_MOUNT:-0}" != "1" ]]; then
  if is_mounted "$DIGITAL_PEOPLE_DIR"; then
    echo "✅ 检测到共享盘已挂载: $DIGITAL_PEOPLE_DIR"
  else
    echo "🔗 尝试挂载共享盘到 $DIGITAL_PEOPLE_DIR ..."
    SHARE_PATH="${SHARE_PATH:-//192.168.7.10/DIGITALPEOPLE}"
    CIFS_USER="${CIFS_USER:-administrator}"
    CIFS_PASS="${CIFS_PASS:-Pddold}"
    CIFS_VERSION="${CIFS_VERSION:-3.0}"
    if [[ -x "$ROOT_DIR/scripts/mount_digitalpeople.sh" ]]; then
      SHARE_PATH="$SHARE_PATH" CIFS_USER="$CIFS_USER" CIFS_PASS="$CIFS_PASS" CIFS_VERSION="$CIFS_VERSION" \
        "$ROOT_DIR/scripts/mount_digitalpeople.sh" "$DIGITAL_PEOPLE_DIR" || { echo "❌ 共享盘挂载失败"; exit 1; }
    else
      echo "⚠️ 未找到 $ROOT_DIR/scripts/mount_digitalpeople.sh，跳过自动挂载"
    fi
  fi
fi

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


