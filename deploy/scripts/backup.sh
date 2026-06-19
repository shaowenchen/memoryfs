#!/usr/bin/env bash
# Backup MemoryFS node data directory (Raft snapshots + metadata + chunks).
set -euo pipefail

NODE_DATA="${1:?usage: $0 <node-data-dir> [backup-dir]}"
BACKUP_DIR="${2:-./backups/memoryfs-$(date +%Y%m%d-%H%M%S)}"

if [ ! -d "${NODE_DATA}" ]; then
  echo "data dir not found: ${NODE_DATA}" >&2
  exit 1
fi

mkdir -p "${BACKUP_DIR}"
ARCHIVE="${BACKUP_DIR}/memoryfs-data.tar.gz"

echo "Backing up ${NODE_DATA} -> ${ARCHIVE}"
tar -czf "${ARCHIVE}" -C "$(dirname "${NODE_DATA}")" "$(basename "${NODE_DATA}")"

cat > "${BACKUP_DIR}/README.txt" <<EOF
MemoryFS backup created at $(date -Iseconds)
Source: ${NODE_DATA}
Archive: ${ARCHIVE}

Restore:
  ./restore.sh ${ARCHIVE} /path/to/restore/data

Notes:
  - Backup includes Raft snapshots and chunk files.
  - For cluster recovery, restore each node PVC/data dir separately.
  - After restore, run node-ready on each node to rebuild missing replicas.
EOF

echo "Backup complete: ${ARCHIVE}"
ls -lh "${ARCHIVE}"
