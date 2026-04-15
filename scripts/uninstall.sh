#!/usr/bin/env bash
set -euo pipefail

TARGET_DIR="${HERMES_GO_HOME:-${HOME}/.hermes-go}"
BIN_DIR="${HERMES_GO_BIN_DIR:-${HOME}/.local/bin}"
PURGE=0

for arg in "$@"; do
  case "$arg" in
    --purge)
      PURGE=1
      ;;
    *)
      echo "unknown option: $arg" >&2
      echo "usage: uninstall.sh [--purge]" >&2
      exit 1
      ;;
  esac
done

rm -f "${BIN_DIR}/hermesd" "${BIN_DIR}/hermesctl"

if [ "${PURGE}" -eq 1 ]; then
  rm -rf "${TARGET_DIR}"
  cat <<EOF
Hermes Go fully uninstalled.
Removed:
  ${BIN_DIR}/hermesd
  ${BIN_DIR}/hermesctl
  ${TARGET_DIR}
EOF
else
  rm -rf "${TARGET_DIR}/bin"
  rm -f "${TARGET_DIR}/QUICKSTART.txt"
  rm -f "${TARGET_DIR}/configs/config.yaml.new"
  cat <<EOF
Hermes Go binaries uninstalled.

Removed:
  ${BIN_DIR}/hermesd
  ${BIN_DIR}/hermesctl
  ${TARGET_DIR}/bin

Kept:
  ${TARGET_DIR}/configs
  ${TARGET_DIR}/data
  ${TARGET_DIR}/plugins
  ${TARGET_DIR}/skills

Use --purge if you also want to remove all configs and data.
EOF
fi
