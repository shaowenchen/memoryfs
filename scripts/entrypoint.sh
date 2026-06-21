#!/bin/sh
set -e

cmd="${1:-}"
if [ -z "$cmd" ]; then
  echo "Usage: memoryfs {node|node-env|mount|status|benchmark} [flags]"
  echo ""
  echo "Examples:"
  echo "  memoryfs node -standalone -id n1 -http :19800 -data /data"
  echo "  memoryfs node-env   # start node from MEMORYFS_* env vars"
  echo "  memoryfs mount -mount /mnt/memoryfs -nodes http://<host>:19800 -f"
  echo "  memoryfs status -nodes http://127.0.0.1:19800"
  echo "  memoryfs benchmark -nodes http://127.0.0.1:19800 -writes 50 -reads 50"
  exit 1
fi

shift
case "$cmd" in
  node)
    exec /app/node "$@"
    ;;
  node-env)
    exec /app/scripts/node-start.sh
    ;;
  mount)
    exec /app/mount "$@"
    ;;
  status)
    exec /app/status "$@"
    ;;
  benchmark)
    exec /app/benchmark "$@"
    ;;
  *)
    echo "unknown command: $cmd (expected node, node-env, mount, status, or benchmark)"
    exit 1
    ;;
esac
