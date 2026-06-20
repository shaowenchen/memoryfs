#!/bin/sh
# Start memoryfs node from environment variables (Kubernetes / node-env entrypoint).
set -e

ID="${MEMORYFS_ID:-n1}"
DATA="${MEMORYFS_DATA:-/data}"
HTTP_LISTEN="${MEMORYFS_HTTP_LISTEN:-:8080}"
GRPC_LISTEN="${MEMORYFS_GRPC_LISTEN:-:9090}"
RDMA_LISTEN="${MEMORYFS_RDMA_LISTEN:-:9092}"
RAFT_LISTEN="${MEMORYFS_RAFT_LISTEN:-:8081}"
CHUNK_DIR="${MEMORYFS_CHUNK_DIR:-${DATA}/chunks}"
CHUNK_BACKEND="${MEMORYFS_CHUNK_BACKEND:-tiered}"
REPLICA_FACTOR="${MEMORYFS_REPLICA_FACTOR:-2}"
MEM_CACHE_MB="${MEMORYFS_MEM_CACHE_MB:-512}"
DISK_QUOTA_GB="${MEMORYFS_DISK_QUOTA_GB:-0}"
GC_INTERVAL="${MEMORYFS_GC_INTERVAL:-5m}"
FLUSH_INTERVAL="${MEMORYFS_FLUSH_INTERVAL:-30s}"
DEFAULT_TTL="${MEMORYFS_DEFAULT_TTL:-0}"
MAX_FILE_AGE="${MEMORYFS_MAX_FILE_AGE:-0}"
API_TOKEN="${MEMORYFS_API_TOKEN:-}"
URI_PREFIX="${MEMORYFS_URI_PREFIX:-}"
BOOTSTRAP="${MEMORYFS_BOOTSTRAP:-false}"
STANDALONE="${MEMORYFS_STANDALONE:-false}"
JOIN="${MEMORYFS_JOIN:-}"

if [ -n "${MEMORYFS_HTTP_URL:-}" ]; then
  ADVERTISE_HTTP="${MEMORYFS_HTTP_URL}"
elif [ -n "${POD_NAME:-}" ] && [ -n "${MEMORYFS_HEADLESS_SERVICE:-}" ]; then
  NS="${POD_NAMESPACE:-default}"
  ADVERTISE_HTTP="http://${POD_NAME}.${MEMORYFS_HEADLESS_SERVICE}.${NS}.svc.cluster.local:8080"
else
  case "${HTTP_LISTEN}" in
    :*) ADVERTISE_HTTP="http://127.0.0.1${HTTP_LISTEN}" ;;
    *)  ADVERTISE_HTTP="http://${HTTP_LISTEN}" ;;
  esac
fi

if [ -n "${MEMORYFS_RAFT_URL:-}" ]; then
  ADVERTISE_RAFT="${MEMORYFS_RAFT_URL}"
elif [ -n "${POD_NAME:-}" ] && [ -n "${MEMORYFS_HEADLESS_SERVICE:-}" ]; then
  NS="${POD_NAMESPACE:-default}"
  ADVERTISE_RAFT="${POD_NAME}.${MEMORYFS_HEADLESS_SERVICE}.${NS}.svc.cluster.local:8081"
else
  ADVERTISE_RAFT="${RAFT_LISTEN}"
fi

set -- node \
  -id "${ID}" \
  -http "${HTTP_LISTEN}" \
  -advertise-http "${ADVERTISE_HTTP}" \
  -grpc "${GRPC_LISTEN}" \
  -rdma "${RDMA_LISTEN}" \
  -raft "${RAFT_LISTEN}" \
  -advertise-raft "${ADVERTISE_RAFT}" \
  -data "${DATA}" \
  -chunk-dir "${CHUNK_DIR}" \
  -chunk-backend "${CHUNK_BACKEND}" \
  -replica-factor "${REPLICA_FACTOR}" \
  -mem-cache-mb "${MEM_CACHE_MB}" \
  -disk-quota-gb "${DISK_QUOTA_GB}" \
  -gc-interval "${GC_INTERVAL}" \
  -flush-interval "${FLUSH_INTERVAL}" \
  -default-ttl "${DEFAULT_TTL}" \
  -max-file-age "${MAX_FILE_AGE}"

[ "${BOOTSTRAP}" = "true" ] && set -- "$@" -bootstrap
[ "${STANDALONE}" = "true" ] && set -- "$@" -standalone
[ -n "${JOIN}" ] && set -- "$@" -join "${JOIN}"
[ -n "${API_TOKEN}" ] && set -- "$@" -api-token "${API_TOKEN}"
[ -n "${URI_PREFIX}" ] && set -- "$@" -uri-prefix "${URI_PREFIX}"

exec /app/node "$@"
