#!/usr/bin/env bash
# Shared helpers for MemoryFS cluster operations.
set -euo pipefail

: "${MEMORYFS_CURL_TIMEOUT:=15}"
: "${MEMORYFS_RETRY:=30}"
: "${MEMORYFS_RETRY_INTERVAL:=2}"

mf_curl() {
  curl -sfS --max-time "${MEMORYFS_CURL_TIMEOUT}" "$@"
}

mf_leader() {
  local node="${1:?node url required}"
  mf_curl "${node%/}/v1/cluster/leader" | sed -n 's/.*"leader"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p'
}

mf_nodes() {
  local node="${1:?node url required}"
  mf_curl -X POST "${node%/}/v1/cluster/nodes" -H 'Content-Type: application/json' -d '{}' \
    | tr ',' '\n' | sed -n 's/.*"\(http[^"]*\)".*/\1/p'
}

mf_wait_state() {
  local node="${1:?}"
  local want="${2:?}"
  local i=0
  while [ "$i" -lt "$MEMORYFS_RETRY" ]; do
    state="$(mf_curl "${node%/}/health" | sed -n 's/.*"node_state"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' || true)"
    if [ "$state" = "$want" ]; then
      return 0
    fi
    i=$((i + 1))
    sleep "$MEMORYFS_RETRY_INTERVAL"
  done
  echo "timeout waiting for ${node} state=${want} (last=${state:-unknown})" >&2
  return 1
}

mf_drain() {
  local node="${1:?}"
  local force="${2:-false}"
  mf_curl -X POST "${node%/}/v1/lifecycle/drain" \
    -H 'Content-Type: application/json' \
    -d "{\"force\":${force}}"
}

mf_ready() {
  local node="${1:?}"
  mf_curl -X POST "${node%/}/v1/lifecycle/ready" -H 'Content-Type: application/json' -d '{}'
}

mf_leave() {
  local node="${1:?}"
  mf_curl -X POST "${node%/}/v1/cluster/leave" -H 'Content-Type: application/json' -d '{}'
}

mf_remove_node() {
  local leader="${1:?}"
  local id="${2:?}"
  mf_curl -X POST "${leader%/}/v1/cluster/remove" \
    -H 'Content-Type: application/json' \
    -d "{\"id\":\"${id}\"}"
}

mf_stats() {
  local node="${1:?}"
  mf_curl "${node%/}/v1/stats"
}

mf_gc() {
  local node="${1:?}"
  mf_curl -X POST "${node%/}/v1/gc" -H 'Content-Type: application/json' -d '{}'
}

mf_join() {
  local leader="${1:?}"
  local id="${2:?}"
  local raft_addr="${3:?}"
  local http_addr="${4:?}"
  local grpc_addr="${5:-}"
  local rdma_addr="${6:-}"
  mf_curl -X POST "${leader%/}/v1/cluster/join" \
    -H 'Content-Type: application/json' \
    -d "{\"id\":\"${id}\",\"raft_addr\":\"${raft_addr}\",\"http_addr\":\"${http_addr}\",\"grpc_addr\":\"${grpc_addr}\",\"rdma_addr\":\"${rdma_addr}\"}"
}
