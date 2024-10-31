#!/bin/bash

if [ -n "${BUILD_DIR}" ]; then
    build="$(cygpath -u "${BUILD_DIR}")"
elif [ -n "${OSTYPE}" ]; then
    if [ -n "${MSYSTEM}" ]; then
        build="${REPO_ROOT}/build-${OSTYPE}-${MSYSTEM}"
    else
        build="${REPO_ROOT}/build-${OSTYPE}"
    fi
elif [ "$USER" = "vk" ]; then
    build="${REPO_ROOT}/build"
else
    # shellcheck disable=SC2034
    build="${REPO_ROOT}/build"
fi
