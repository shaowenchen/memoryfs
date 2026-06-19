#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

NODE="${1:?usage: $0 <node-http-url> [force=true]}"
FORCE="${2:-false}"

echo "Draining ${NODE} (force=${FORCE})..."
mf_drain "${NODE}" "${FORCE}"
mf_wait_state "${NODE}" "drained"
echo "Node drained."
