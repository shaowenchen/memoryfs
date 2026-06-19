#!/usr/bin/env bash
# Gracefully remove a node from the cluster (drain + leave).
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

NODE="${1:?usage: $0 <node-http-url>}"
SEED="${2:-${NODE}}"

echo "Scale-down: removing ${NODE}"
mf_leave "${NODE}"
echo "Removed from cluster. Stop the node process/Pod now."
echo
echo "Remaining nodes:"
"${SCRIPT_DIR}/cluster-status.sh" "${SEED}"
