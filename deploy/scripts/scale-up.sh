#!/usr/bin/env bash
# Scale up MemoryFS cluster by starting a new node and joining the leader.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

LEADER="${1:?usage: $0 <leader-http> <node-id> <raft-addr> <http-addr> [grpc-addr]}"
ID="${2:?}"
RAFT="${3:?}"
HTTP="${4:?}"
GRPC="${5:-}"
RDMA="${6:-}"

echo "Joining node ${ID} to cluster via ${LEADER}..."
mf_join "${LEADER}" "${ID}" "${RAFT}" "${HTTP}" "${GRPC}" "${RDMA}"
echo "Joined. Waiting for node to become active..."
mf_ready "${HTTP}"
mf_wait_state "${HTTP}" "active"
echo "Scale-up complete: ${ID} @ ${HTTP}"
