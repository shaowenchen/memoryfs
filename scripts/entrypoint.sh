#!/bin/sh
set -e

cmd="${1:-}"
if [ -z "$cmd" ]; then
  echo "Usage: memoryfs {node|node-env|mount|status|benchmark|config} [flags]"
  echo ""
  echo "Examples:"
  echo "  memoryfs node -standalone -id n1 -http :19800 -data /data"
  echo "  memoryfs node-env   # start node from MEMORYFS_* env vars"
  echo "  memoryfs mount -mount /mnt/memoryfs -nodes http://<host>:19800 -f"
  echo "  memoryfs status     # reuses nodes saved by mount"
  echo "  memoryfs benchmark  # reuses nodes saved by mount"
  echo "  memoryfs config show"
  exit 1
fi

shift
case "$cmd" in
  node-env)
    exec /app/scripts/node-start.sh
    ;;
  node|mount|status|benchmark|config|version|help)
    exec /app/memoryfs "$cmd" "$@"
    ;;
  *)
    echo "unknown command: $cmd (expected node, node-env, mount, status, benchmark, or config)"
    exit 1
    ;;
esac
