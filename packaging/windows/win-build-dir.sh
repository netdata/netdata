#!/bin/bash

if [ -n "${BUILD_DIR}" ]; then
    if (echo "${BUILD_DIR}" | grep -q -E "^[A-Z]:\\\\"); then
        build="$(echo "${BUILD_DIR}" | sed -e 's/\\/\//g' -e 's/^\([A-Z]\):\//\/\1\//' -)"
    else
        build="${BUILD_DIR}"
    fi
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
