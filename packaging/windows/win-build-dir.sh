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

