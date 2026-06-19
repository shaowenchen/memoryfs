#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

NODE="${1:?usage: $0 <node-http-url>}"

echo "Running GC on ${NODE}..."
mf_gc "${NODE}"
echo "Done."
