#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

NODE="${1:?usage: $0 <node-http-url>}"

echo "Graceful leave: drain + remove from cluster..."
mf_leave "${NODE}"
echo "Node left cluster."
