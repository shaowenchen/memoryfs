#!/usr/bin/env bash
# Restore MemoryFS node data from backup archive.
set -euo pipefail

ARCHIVE="${1:?usage: $0 <backup.tar.gz> <target-data-dir>}"
TARGET="${2:?usage: $0 <backup.tar.gz> <target-data-dir>}"

if [ ! -f "${ARCHIVE}" ]; then
  echo "archive not found: ${ARCHIVE}" >&2
  exit 1
fi

mkdir -p "${TARGET}"
echo "Restoring ${ARCHIVE} -> ${TARGET}"
tar -xzf "${ARCHIVE}" -C "${TARGET}" --strip-components=0

echo "Restore complete."
echo "Start node and run: deploy/scripts/node-ready.sh <node-http-url>"
