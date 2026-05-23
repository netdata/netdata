#!/usr/bin/env bash

set -euo pipefail

RED='\033[0;31m'
YELLOW='\033[1;33m'
GRAY='\033[0;90m'
NC='\033[0m'
TOPOLOGY_IP_INTEL_STOCK_TMPDIR=""

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

cleanup_topology_ip_intel_stock_tmpdir() {
  if [ -n "${TOPOLOGY_IP_INTEL_STOCK_TMPDIR:-}" ]; then
    rm -rf -- "${TOPOLOGY_IP_INTEL_STOCK_TMPDIR}"
  fi
}

trap cleanup_topology_ip_intel_stock_tmpdir EXIT

usage() {
  cat <<'EOF'
Usage: prepare-topology-ip-intel-stock.sh --mode synthetic|release --output-dir DIR

Modes:
  synthetic  Create a tiny deterministic ASN/GEO stock payload for PR package checks.
  release    Build the real DB-IP ASN + GEO stock payload used for package/release builds.
EOF
}

MODE=""
OUTPUT_DIR=""

while [ "$#" -gt 0 ]; do
  case "$1" in
    --mode)
      MODE="${2:-}"
      shift 2
      ;;
    --output-dir)
      OUTPUT_DIR="${2:-}"
      shift 2
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      printf >&2 "Unknown argument: %s\n" "$1"
      usage >&2
      exit 1
      ;;
  esac
done

if [ -z "${MODE}" ] || [ -z "${OUTPUT_DIR}" ]; then
  usage >&2
  exit 1
fi

case "${OUTPUT_DIR}" in
  /*) ;;
  *)
    OUTPUT_DIR="$(pwd)/${OUTPUT_DIR}"
    ;;
esac

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/../.." && pwd)"
DOWNLOADER_DIR="${REPO_ROOT}/src/go"
DOWNLOADER_CONFIG="${REPO_ROOT}/src/go/tools/topology-ip-intel-downloader/configs/topology-ip-intel.yaml"
STOCK_README="${REPO_ROOT}/src/go/tools/topology-ip-intel-downloader/stock/README.md"

case "${MODE}" in
  synthetic|release) ;;
  *)
    printf >&2 "Unsupported mode: %s\n" "${MODE}"
    usage >&2
    exit 1
    ;;
esac

run mkdir -p "${OUTPUT_DIR}"
run rm -f \
  "${OUTPUT_DIR}/README.md" \
  "${OUTPUT_DIR}/topology-ip-asn.mmdb" \
  "${OUTPUT_DIR}/topology-ip-geo.mmdb" \
  "${OUTPUT_DIR}/topology-ip-intel.json"

prepare_synthetic() {
  local tmpdir
  tmpdir="$(mktemp -d)"
  TOPOLOGY_IP_INTEL_STOCK_TMPDIR="${tmpdir}"

  run tee "${tmpdir}/asn.csv" >/dev/null <<'EOF'
1.1.1.0,1.1.1.255,13335,"Cloudflare, Inc."
8.8.8.0,8.8.8.255,15169,Google LLC
203.0.113.0,203.0.113.255,64500,Example Transit
EOF

  run tee "${tmpdir}/geo.csv" >/dev/null <<'EOF'
1.1.1.0,1.1.1.255,,AU,Queensland,South Brisbane,-27.4748,153.0170
8.8.8.0,8.8.8.255,,US,California,Mountain View,37.4220,-122.0850
203.0.113.0,203.0.113.255,,US,Virginia,Ashburn,39.0438,-77.4874
EOF

  run tee "${tmpdir}/topology-ip-intel.yaml" >/dev/null <<EOF
sources:
  - name: pr-synthetic-asn
    family: asn
    provider: dbip
    artifact: asn-lite
    format: csv
    path: ${tmpdir}/asn.csv
  - name: pr-synthetic-geo
    family: geo
    provider: dbip
    artifact: city-lite
    format: csv
    path: ${tmpdir}/geo.csv
EOF

  cd "${DOWNLOADER_DIR}"
  run go run ./tools/topology-ip-intel-downloader --config "${tmpdir}/topology-ip-intel.yaml" --output-dir "${OUTPUT_DIR}"
}

prepare_release() {
  cd "${DOWNLOADER_DIR}"
  run go run ./tools/topology-ip-intel-downloader --config "${DOWNLOADER_CONFIG}" --output-dir "${OUTPUT_DIR}"
}

case "${MODE}" in
  synthetic)
    prepare_synthetic
    ;;
  release)
    prepare_release
    ;;
esac

run cp "${STOCK_README}" "${OUTPUT_DIR}/README.md"
