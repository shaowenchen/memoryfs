#!/usr/bin/env bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

SEED="${1:-http://127.0.0.1:8080}"

echo "== MemoryFS cluster status =="
echo "Seed node: ${SEED}"
echo

leader="$(mf_leader "${SEED}" || true)"
echo "Leader: ${leader:-unknown}"
echo

echo "Nodes:"
for n in $(mf_nodes "${SEED}" 2>/dev/null || true); do
  echo "--- ${n}"
  mf_curl "${n}/health" 2>/dev/null || echo "  unreachable"
  mf_stats "${n}" 2>/dev/null || true
  echo
done
