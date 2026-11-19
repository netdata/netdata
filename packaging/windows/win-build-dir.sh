#!/usr/bin/env bash

if [[ -z "${REPO_ROOT:-}" ]]; then
    REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
fi

if [[ -n "${BUILD_DIR:-}" ]]; then
    build="$(cygpath -u "${BUILD_DIR}")"
    if [[ "${build}" != /* ]]; then
        build="${REPO_ROOT}/${build}"
    fi
else
    build="${REPO_ROOT}/build"
fi

# Select the UCRT64 toolchain from any MSYS2 terminal, so callers need no manual `export MSYSTEM=UCRT64`.
netdata_use_ucrt64() {
    if ! command -v cygpath > /dev/null 2>&1 || [[ ! -d /ucrt64/bin ]]; then
        echo "MSYS2 UCRT64 was not found. Run this script from an MSYS2 terminal with the UCRT64 toolchain installed." >&2
        exit 1
    fi
    if [[ "${MSYSTEM:-}" != "UCRT64" ]]; then
        echo "Setting MSYSTEM=UCRT64 (was '${MSYSTEM:-unset}')." >&2
        export MSYSTEM="UCRT64"
    fi
    export PATH="/ucrt64/bin:/ucrt64/sbin:${PATH}"
}
