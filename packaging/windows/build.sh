#!/bin/bash

. /etc/profile

set -e

repo_root="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"

BUILD_DIR="${BUILD_DIR:-"${repo_root}/build"}"
BUILD_TYPE="${BUILD_TYPE:-RelWithDebSymbols}"
CFLAGS="${CFLAGS} -D_FILE_OFFSET_BITS=64 -D__USE_MINGW_ANSI_STDIO=1"

cmake -S "${repo_root}" -B "${BUILD_DIR}" -G Ninja \
    -DCMAKE_INSTALL_PREFIX="/opt/netdata" \
    -DCMAKE_BUILD_TYPE="${BUILD_TYPE}" \
    -DBUILD_FOR_PACKAGING=On \
    -DDEFAULT_FEATURE_STATE=Off \
    -DENABLE_H2O=Off \
    -DENABLE_ACLK=On \
    -DENABLE_CLOUD=On \
    -DENABLE_ML=On

cmake --build "${BUILD_DIR}" -- -k 1
