#!/usr/bin/env bash
set -euo pipefail

SHARE_PATH="${SHARE_PATH:-//192.168.7.10/DIGITALPEOPLE}"
MOUNT_POINT="${1:-/mnt/windows-digitalpeople}"
CIFS_USER="${CIFS_USER:-administrator}"
CIFS_PASS="${CIFS_PASS:-Pddold}"
CIFS_VERSION="${CIFS_VERSION:-3.0}"

if ! command -v mount.cifs >/dev/null 2>&1; then
  echo "[INFO] 未检测到 cifs-utils，请先运行: sudo apt update && sudo apt install -y cifs-utils" >&2
  exit 1
fi

echo "[INFO] 准备挂载 $SHARE_PATH 到 $MOUNT_POINT"

sudo mkdir -p "$MOUNT_POINT"

echo "[INFO] 挂载中..."
sudo mount -t cifs "$SHARE_PATH" "$MOUNT_POINT" \
  -o "username=$CIFS_USER,password=$CIFS_PASS,uid=$(id -u),gid=$(id -g),vers=$CIFS_VERSION"

echo "[INFO] 挂载完成"

REQUIRED_DIRS=(
  "voice/data"
  "face2face"
  "face2face/temp"
  "face2face/result"
  "workdir"
)

for dir in "${REQUIRED_DIRS[@]}"; do
  target="$MOUNT_POINT/$dir"
  echo "[INFO] 创建目录: $target"
  mkdir -p "$target"
done
