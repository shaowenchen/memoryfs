#!/usr/bin/env bash
# FUSE mount helper — expects MEMORYFS_MOUNT_NODES comma-separated HTTP URLs.
set -euo pipefail

: "${MEMORYFS_MOUNT_POINT:=/mnt/memoryfs}"
: "${MEMORYFS_MOUNT_NODES:?MEMORYFS_MOUNT_NODES required}"
: "${MEMORYFS_MOUNT_FOREGROUND:=true}"

args=(
  -mount "${MEMORYFS_MOUNT_POINT}"
  -nodes "${MEMORYFS_MOUNT_NODES}"
  -v
)
if [ "${MEMORYFS_MOUNT_FOREGROUND}" = "true" ]; then
  args+=(-f)
fi

echo "memoryfs mount-start: point=${MEMORYFS_MOUNT_POINT} nodes=${MEMORYFS_MOUNT_NODES}"
exec /app/mount "${args[@]}"
