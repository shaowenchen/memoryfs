#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

NODE="${1:?usage: $0 <node-http-url>}"

echo "Trigger rebuild on ${NODE}..."
mf_ready "${NODE}"
echo "Rebuild triggered."
