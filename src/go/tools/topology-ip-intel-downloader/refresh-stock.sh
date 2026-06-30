#!/usr/bin/env bash

set -euo pipefail

RED='\033[0;31m'
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
  else
    local exit_code=$?
    echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e >&2 "${RED}[ERROR]${NC} Command failed with exit code ${exit_code}: ${YELLOW}$1${NC}"
    echo -e >&2 "${RED}        Full command:${NC} $*"
    echo -e >&2 "${RED}        Working dir:${NC} $(pwd)"
    echo -e >&2 "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    return "${exit_code}"
  fi
}

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../../../.." && pwd)"
OUTPUT_DIR="${1:-${REPO_ROOT}/artifacts/topology-ip-intel-stock}"
PREPARE_SCRIPT="${REPO_ROOT}/.github/scripts/prepare-topology-ip-intel-stock.sh"
if [[ "${OUTPUT_DIR}" != /* ]]; then
  OUTPUT_DIR="$(pwd)/${OUTPUT_DIR}"
fi

run mkdir -p "${OUTPUT_DIR}"
cd "${REPO_ROOT}"
run "${PREPARE_SCRIPT}" --mode release --output-dir "${OUTPUT_DIR}"
