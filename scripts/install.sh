#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TARGET_DIR="${HERMES_GO_HOME:-${HOME}/.hermes-go}"
BIN_DIR="${HERMES_GO_BIN_DIR:-${HOME}/.local/bin}"
APP_BIN_DIR="${TARGET_DIR}/bin"
CONFIG_DIR="${TARGET_DIR}/configs"
DATA_DIR="${TARGET_DIR}/data"
PLUGINS_DIR="${TARGET_DIR}/plugins"
SKILLS_DIR="${TARGET_DIR}/skills"
LOG_DIR="${TARGET_DIR}/logs"
CONFIG_PATH="${CONFIG_DIR}/config.yaml"
CONFIG_NEW_PATH="${CONFIG_DIR}/config.yaml.new"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "missing required command: $1" >&2
    exit 1
  fi
}

write_default_config() {
  cp -f "${ROOT_DIR}/configs/config.example.yaml" "${CONFIG_PATH}"
  perl -0pi -e 's|data_dir: "\./data"|data_dir: "'"${DATA_DIR}"'"|g' "${CONFIG_PATH}"
  perl -0pi -e 's|plugins_dir: "\./plugins"|plugins_dir: "'"${PLUGINS_DIR}"'"|g' "${CONFIG_PATH}"
  perl -0pi -e 's|-\s+"\./skills"|  - "'"${SKILLS_DIR}"'"|g' "${CONFIG_PATH}"
}

show_next_steps() {
  cat <<EOF

Install complete.

Installed binaries:
  ${BIN_DIR}/hermesd
  ${BIN_DIR}/hermesctl

Workspace:
  ${TARGET_DIR}

Main config:
  ${CONFIG_PATH}

Quick start:
  hermesctl version
  hermesctl init-admin --config "${CONFIG_PATH}" --username admin --password 'ChangeMe123!'
  hermesd --config "${CONFIG_PATH}"

Notes:
  - Data will be stored in ${DATA_DIR}
  - Put local plugins under ${PLUGINS_DIR}
  - Put local skills under ${SKILLS_DIR}
EOF

  if [ ! -d "${BIN_DIR}" ] || [[ ":${PATH}:" != *":${BIN_DIR}:"* ]]; then
    cat <<EOF

PATH reminder:
  export PATH="${BIN_DIR}:\$PATH"
EOF
  fi

  if [ -f "${CONFIG_NEW_PATH}" ]; then
    cat <<EOF

Config note:
  Existing config was kept.
  A fresh template was written to:
    ${CONFIG_NEW_PATH}
EOF
  fi
}

need_cmd go
need_cmd perl

mkdir -p "${BIN_DIR}" "${APP_BIN_DIR}" "${CONFIG_DIR}" "${DATA_DIR}" "${PLUGINS_DIR}" "${SKILLS_DIR}" "${LOG_DIR}"

echo "Building hermes-go..."
cd "${ROOT_DIR}"
go mod tidy
go build -o "${APP_BIN_DIR}/hermesd" ./cmd/hermesd
go build -o "${APP_BIN_DIR}/hermesctl" ./cmd/hermesctl

ln -sf "${APP_BIN_DIR}/hermesd" "${BIN_DIR}/hermesd"
ln -sf "${APP_BIN_DIR}/hermesctl" "${BIN_DIR}/hermesctl"

if [ ! -f "${CONFIG_PATH}" ]; then
  write_default_config
else
  cp -f "${ROOT_DIR}/configs/config.example.yaml" "${CONFIG_NEW_PATH}"
  perl -0pi -e 's|data_dir: "\./data"|data_dir: "'"${DATA_DIR}"'"|g' "${CONFIG_NEW_PATH}"
  perl -0pi -e 's|plugins_dir: "\./plugins"|plugins_dir: "'"${PLUGINS_DIR}"'"|g' "${CONFIG_NEW_PATH}"
  perl -0pi -e 's|-\s+"\./skills"|  - "'"${SKILLS_DIR}"'"|g' "${CONFIG_NEW_PATH}"
fi

cat > "${TARGET_DIR}/QUICKSTART.txt" <<EOF
Hermes Go Quickstart
====================

1. Create an admin:
   hermesctl init-admin --config "${CONFIG_PATH}" --username admin --password 'ChangeMe123!'

2. Start the server:
   hermesd --config "${CONFIG_PATH}"

3. Optional login:
   hermesctl login --config "${CONFIG_PATH}" --username admin --password 'ChangeMe123!'

Paths:
  config: ${CONFIG_PATH}
  data:   ${DATA_DIR}
  skills: ${SKILLS_DIR}
  plugins:${PLUGINS_DIR}
EOF

show_next_steps
