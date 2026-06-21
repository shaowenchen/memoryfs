#!/bin/sh
# Start memoryfs node from environment variables (Kubernetes / node-env entrypoint).
set -e

HTTP_LISTEN="${MEMORYFS_HTTP_LISTEN:-:19800}"
GRPC_LISTEN="${MEMORYFS_GRPC_LISTEN:-:19801}"
RDMA_LISTEN="${MEMORYFS_RDMA_LISTEN:-:19803}"
RAFT_LISTEN="${MEMORYFS_RAFT_LISTEN:-:19802}"

if [ -n "${POD_NAME:-}" ]; then
  ID="${POD_NAME}"
elif [ -n "${MEMORYFS_ID:-}" ]; then
  ID="${MEMORYFS_ID}"
else
  ID="n1"
fi

BOOTSTRAP="${MEMORYFS_BOOTSTRAP:-false}"
STANDALONE="${MEMORYFS_STANDALONE:-false}"
JOIN="${MEMORYFS_JOIN:-}"

if [ -n "${POD_NAME:-}" ] && [ "${BOOTSTRAP}" != "true" ] && [ "${STANDALONE}" != "true" ] && [ -z "${JOIN}" ]; then
  _ord="${POD_NAME##*-}"
  if [ "$_ord" = "0" ]; then
    BOOTSTRAP=true
  elif [ -n "${MEMORYFS_HEADLESS_SERVICE:-}" ]; then
    NS="${POD_NAMESPACE:-default}"
    _base="${POD_NAME%-*}"
    _http_port="${HTTP_LISTEN##*:}"
    JOIN="http://${_base}-0.${MEMORYFS_HEADLESS_SERVICE}.${NS}.svc:${_http_port}"
  fi
fi

if [ -n "${MEMORYFS_INSTANCE_ID:-}" ] && [ -n "${MEMORYFS_STORAGE_ROOT:-}" ]; then
  DATA="${MEMORYFS_STORAGE_ROOT}/${MEMORYFS_INSTANCE_ID}"
  NODE_DATA="${DATA}/${ID}"
else
  DATA="${MEMORYFS_DATA:-/data}"
  NODE_DATA="${DATA}/${ID}"
fi
CHUNK_DIR="${MEMORYFS_CHUNK_DIR:-${NODE_DATA}/chunks}"
mkdir -p "${NODE_DATA}" "${CHUNK_DIR}"

if [ "${MEMORYFS_RAFT_RESET:-}" = "true" ]; then
  echo "memoryfs: MEMORYFS_RAFT_RESET=true, clearing raft state in ${NODE_DATA}"
  rm -rf "${NODE_DATA}/raft.db" "${NODE_DATA}/snapshots"
fi

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

_http_port="${HTTP_LISTEN##*:}"
_raft_port="${RAFT_LISTEN##*:}"
_grpc_port="${GRPC_LISTEN##*:}"
_rdma_port="${RDMA_LISTEN##*:}"

if [ -n "${MEMORYFS_HTTP_URL:-}" ]; then
  ADVERTISE_HTTP="${MEMORYFS_HTTP_URL}"
elif [ -n "${MEMORYFS_HOST_IP:-}" ]; then
  ADVERTISE_HTTP="http://${MEMORYFS_HOST_IP}:${_http_port}"
  ADVERTISE_RAFT="${MEMORYFS_HOST_IP}:${_raft_port}"
  RAFT_LISTEN="${MEMORYFS_HOST_IP}:${_raft_port}"
  GRPC_LISTEN="${MEMORYFS_HOST_IP}:${_grpc_port}"
  RDMA_LISTEN="${MEMORYFS_HOST_IP}:${_rdma_port}"
elif [ -n "${POD_NAME:-}" ] && [ -n "${MEMORYFS_HEADLESS_SERVICE:-}" ]; then
  NS="${POD_NAMESPACE:-default}"
  ADVERTISE_HTTP="http://${POD_NAME}.${MEMORYFS_HEADLESS_SERVICE}.${NS}.svc.cluster.local:${_http_port}"
  ADVERTISE_RAFT="${POD_NAME}.${MEMORYFS_HEADLESS_SERVICE}.${NS}.svc.cluster.local:${_raft_port}"
else
  case "${HTTP_LISTEN}" in
    :*) ADVERTISE_HTTP="http://127.0.0.1${HTTP_LISTEN}" ;;
    *)  ADVERTISE_HTTP="http://${HTTP_LISTEN}" ;;
  esac
  ADVERTISE_RAFT="${RAFT_LISTEN}"
fi

if [ -n "${MEMORYFS_RAFT_URL:-}" ]; then
  ADVERTISE_RAFT="${MEMORYFS_RAFT_URL}"
fi

echo "memoryfs node-env: id=${ID} bootstrap=${BOOTSTRAP} join=${JOIN:-<none>} data=${NODE_DATA} advertise_http=${ADVERTISE_HTTP} advertise_raft=${ADVERTISE_RAFT} raft_listen=${RAFT_LISTEN}"

set -- \
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
