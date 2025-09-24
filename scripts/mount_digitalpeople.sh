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

TMP_TEST_FILE="$MOUNT_POINT/.writable_check_$$"
if ! touch "$TMP_TEST_FILE" 2>/dev/null; then
  echo "[ERROR] 无法在 $MOUNT_POINT 写入。请检查共享盘权限或凭证 (CIFS_USER/CIFS_PASS/CIFS_VERSION)。" >&2
  echo "[HINT] 可以手动在 Windows 端创建所需目录，或在执行脚本前修改上述环境变量。" >&2
  sudo umount "$MOUNT_POINT" >/dev/null 2>&1 || true
  exit 1
fi
rm -f "$TMP_TEST_FILE"

REQUIRED_DIRS=(
  "voice/data"
  "face2face"
  "face2face/temp"
  "face2face/result"
  "workdir"
)

for dir in "${REQUIRED_DIRS[@]}"; do
  target="$MOUNT_POINT/$dir"
  if [[ -d "$target" ]]; then
    echo "[INFO] 目录已存在: $target"
    continue
  fi
  echo "[INFO] 创建目录: $target"
  if ! mkdir -p "$target"; then
    echo "[ERROR] 创建目录失败: $target" >&2
    echo "[HINT] 请确认共享盘对用户 $CIFS_USER 可写，或手动预先创建这些目录。" >&2
    sudo umount "$MOUNT_POINT" >/dev/null 2>&1 || true
    exit 1
  fi
done
