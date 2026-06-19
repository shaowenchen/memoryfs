#!/usr/bin/env bash
# Rolling update: drain followers first, then leader last.
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=common.sh
source "${SCRIPT_DIR}/common.sh"

SEED="${1:?usage: $0 <any-node-http-url>}"

leader="$(mf_leader "${SEED}")"
echo "Leader: ${leader}"

mapfile -t nodes < <(mf_nodes "${SEED}")
followers=()
for n in "${nodes[@]}"; do
  if [ "${n}" != "${leader}" ]; then
    followers+=("${n}")
  fi
done

echo "Updating followers first (${#followers[@]})..."
for n in "${followers[@]}"; do
  echo "== drain ${n}"
  mf_drain "${n}" false
  mf_wait_state "${n}" "drained"
  echo "   -> stop/restart this node, then run node-ready"
  read -r -p "Press enter after node restarted and ready..."
  mf_ready "${n}"
  mf_wait_state "${n}" "active"
done

echo "== drain leader ${leader}"
mf_drain "${leader}" false
mf_wait_state "${leader}" "drained"
echo "   -> stop/restart leader, then run node-ready"
read -r -p "Press enter after leader restarted and ready..."
mf_ready "${leader}"
mf_wait_state "${leader}" "active"

echo "Rolling update complete."
"${SCRIPT_DIR}/cluster-status.sh" "${SEED}"
