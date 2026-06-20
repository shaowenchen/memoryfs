#!/usr/bin/env bash
# Remove stale MemoryFS FUSE mount points on the host.
# Run on the node where nerdctl mount containers were started.
set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "usage: $0 /path/to/mount [/another/mount ...]" >&2
  echo "example: $0 /memoryfs /memoryfs1 /memoryfs2 /mnt/memoryfs" >&2
  exit 1
fi

for mp in "$@"; do
  if mountpoint -q "$mp" 2>/dev/null; then
    echo "unmounting stale mount: $mp"
    if fusermount -u "$mp" 2>/dev/null || fusermount3 -u "$mp" 2>/dev/null; then
      echo "ok: $mp"
    else
      echo "retry lazy unmount: $mp"
      fusermount -uz "$mp" 2>/dev/null || fusermount3 -uz "$mp" 2>/dev/null || true
    fi
  else
    echo "skip (not mounted): $mp"
  fi
done
