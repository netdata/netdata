#!/usr/bin/env bash
# run_test.sh — build and run test_spawn_python on MSYS2 UCRT64
#
# Prerequisites:
#   - Full cmake build already done (cmake --build build/ ...)
#   - MSYS2 UCRT64 shell
#   - Python installed (python3.exe or python.exe in PATH for test 1 & 2)
#
# Usage:
#   cd <repo_root>
#   bash tests/windows/spawn_python/run_test.sh [BUILD_DIR]
#
#   BUILD_DIR defaults to ./build

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../../.." && pwd)"
BUILD_DIR="${1:-${REPO_ROOT}/build}"

echo "Repo root : ${REPO_ROOT}"
echo "Build dir : ${BUILD_DIR}"
echo ""

# ---------------------------------------------------------------------------
# locate libnetdata
# ---------------------------------------------------------------------------
LIBNETDATA=""
for candidate in \
    "${BUILD_DIR}/src/libnetdata/libnetdata.a" \
    "${BUILD_DIR}/src/libnetdata/libnetdata_static.a" \
    "${BUILD_DIR}/libnetdata.a"
do
    if [ -f "${candidate}" ]; then
        LIBNETDATA="${candidate}"
        break
    fi
done

if [ -z "${LIBNETDATA}" ]; then
    echo "ERROR: cannot find libnetdata.a under ${BUILD_DIR}."
    echo "       Run the cmake build first."
    exit 1
fi
echo "libnetdata: ${LIBNETDATA}"

# ---------------------------------------------------------------------------
# compile
# ---------------------------------------------------------------------------
OUT="${SCRIPT_DIR}/test_spawn_python.exe"
echo "Compiling  -> ${OUT}"
gcc -std=gnu11 \
    -DOS_WINDOWS \
    -DNETDATA_INTERNAL_CHECKS \
    -DNETDATA_DEV_MODE \
    -o "${OUT}" \
    "${SCRIPT_DIR}/test_spawn_python.c" \
    -I "${REPO_ROOT}/src" \
    "${LIBNETDATA}" \
    -lws2_32 -lpthread -lshlwapi -ladvapi32 -luserenv

echo "Compiled OK"
echo ""

# ---------------------------------------------------------------------------
# run from the test directory so test_plugin.py is found
# ---------------------------------------------------------------------------
cd "${SCRIPT_DIR}"
echo "Running test_spawn_python.exe ..."
echo ""
"${OUT}"
