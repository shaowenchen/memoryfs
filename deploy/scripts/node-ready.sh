#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

NODE="${1:?usage: $0 <node-http-url>}"

echo "Marking ${NODE} ready and rebuilding chunks..."
mf_ready "${NODE}"
mf_wait_state "${NODE}" "active"
echo "Node active."
