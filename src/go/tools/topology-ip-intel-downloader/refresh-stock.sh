#!/usr/bin/env bash

set -euo pipefail

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m'

run() {
  printf >&2 "${GRAY}%s >${NC} " "$(pwd)"
  printf >&2 "${YELLOW}"
  printf >&2 "%q " "$@"
  printf >&2 "${NC}\n"

  if "$@"; then
    return 0
  fi

  local exit_code=$?
  echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  echo -e >&2 "${RED}[ERROR]${NC} Command failed with exit code ${exit_code}: ${YELLOW}$1${NC}"
  echo -e >&2 "${RED}        Full command:${NC} $*"
  echo -e >&2 "${RED}        Working dir:${NC} $(pwd)"
  echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
  return "${exit_code}"
}

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../../../.." && pwd)"
OUTPUT_DIR="${SCRIPT_DIR}/stock/topology-ip-intel"
CONFIG_PATH="${SCRIPT_DIR}/configs/topology-ip-intel.yaml"

run mkdir -p "${OUTPUT_DIR}"
cd "${REPO_ROOT}/src/go"
run go run ./tools/topology-ip-intel-downloader --config "${CONFIG_PATH}" --output-dir "${OUTPUT_DIR}"
