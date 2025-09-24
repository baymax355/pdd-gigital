#!/usr/bin/env bash
set -euo pipefail

show_help() {
  cat <<'USAGE'
用法: ./scripts/auto_setup_and_start.sh [选项]

选项:
  --mount DIR          指定共享盘挂载点（默认 /mnt/windows-digitalpeople）
  --skip-mount         不执行挂载脚本，仅使用已挂载的目录
  --start-mode MODE    传递给 start.sh 的模式（web 或 all，默认 all）
  --start-arg ARG      追加额外参数给 start.sh，可重复多次
  -h, --help           显示本帮助

环境变量:
  SHARE_PATH, CIFS_USER, CIFS_PASS, CIFS_VERSION 会传递给挂载脚本。
USAGE
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MOUNT_POINT="/mnt/windows-digitalpeople"
DO_MOUNT=1
START_MODE="all"
START_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --mount)
      shift
      [[ $# -gt 0 ]] || { echo "缺少 --mount 参数" >&2; exit 1; }
      MOUNT_POINT="$1"
      ;;
    --skip-mount)
      DO_MOUNT=0
      ;;
    --start-mode)
      shift
      [[ $# -gt 0 ]] || { echo "缺少 --start-mode 参数" >&2; exit 1; }
      START_MODE="$1"
      ;;
    --start-arg)
      shift
      [[ $# -gt 0 ]] || { echo "缺少 --start-arg 参数" >&2; exit 1; }
      START_ARGS+=("$1")
      ;;
    -h|--help)
      show_help
      exit 0
      ;;
    *)
      echo "未知参数: $1" >&2
      show_help
      exit 1
      ;;
  esac
  shift
done

if [[ "$DO_MOUNT" -eq 1 ]]; then
  if mountpoint -q "$MOUNT_POINT"; then
    echo "[INFO] 挂载点 $MOUNT_POINT 已挂载，跳过挂载。"
  else
    echo "[INFO] 调用挂载脚本挂载共享盘..."
    "$SCRIPT_DIR/mount_digitalpeople.sh" "$MOUNT_POINT"
  fi
else
  echo "[INFO] 已跳过挂载步骤。"
fi

export DIGITAL_PEOPLE_DIR="$MOUNT_POINT"
export APP_WORKDIR="$DIGITAL_PEOPLE_DIR/workdir"
export HOST_VOICE_DIR="$DIGITAL_PEOPLE_DIR/voice/data"
export HOST_VIDEO_DIR="$DIGITAL_PEOPLE_DIR/face2face"
export HOST_RESULT_DIR="$DIGITAL_PEOPLE_DIR/face2face/result"
export WIN_COMPANY_DIR="$DIGITAL_PEOPLE_DIR"

cd "$REPO_ROOT"

echo "[INFO] 启动模式: $START_MODE"
if [[ ${#START_ARGS[@]} -gt 0 ]]; then
  echo "[INFO] 附加 start.sh 参数: ${START_ARGS[*]}"
fi

./start.sh "$START_MODE" "${START_ARGS[@]}"
