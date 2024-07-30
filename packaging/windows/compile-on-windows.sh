#!/bin/bash

repo_root="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"
CMAKE_BUILD_TYPE="${CMAKE_BUILD_TYPE:-RelWithDebInfo}"

if [ -n "${BUILD_DIR}" ]; then
    build="${BUILD_DIR}"
elif [ -n "${OSTYPE}" ]; then
    if [ -n "${MSYSTEM}" ]; then
        build="${repo_root}/build-${OSTYPE}-${MSYSTEM}"
    else
        build="${repo_root}/build-${OSTYPE}"
    fi
elif [ "$USER" = "vk" ]; then
    build="${repo_root}/build"
else
    build="${repo_root}/build"
fi

set -exu -o pipefail

if [ -d "${build}" ]; then
	rm -rf "${build}"
fi

generator="Unix Makefiles"
build_args="-j $(nproc)"

if command -v ninja >/dev/null 2>&1; then
    generator="Ninja"
    build_args="-k 1"
fi

COMMON_CFLAGS="-Wa,-mbig-obj -pipe -D_FILE_OFFSET_BITS=64 -D__USE_MINGW_ANSI_STDIO=1"

if [ "${CMAKE_BUILD_TYPE}" = "Debug" ]; then
    BUILD_CFLAGS="-fstack-protector-all -O0 -ggdb -Wall -Wextra -Wno-char-subscripts -DNETDATA_INTERNAL_CHECKS=1 ${COMMON_CFLAGS} ${CFLAGS:-}"
else
    BUILD_CFLAGS="-O2 ${COMMON_CFLAGS} ${CFLAGS:-}"
fi

${GITHUB_ACTIONS+echo "::group::Configuring"}
# shellcheck disable=SC2086
CFLAGS="${BUILD_CFLAGS}" /usr/bin/cmake \
    -S "${repo_root}" \
    -B "${build}" \
    -G "${generator}" \
    -DCMAKE_INSTALL_PREFIX="/opt/netdata" \
    -DBUILD_FOR_PACKAGING=On \
    -DNETDATA_USER="${USER}" \
    -DENABLE_ACLK=On \
    -DENABLE_CLOUD=On \
    -DENABLE_H2O=Off \
    -DENABLE_ML=On \
    -DENABLE_PLUGIN_GO=On \
    -DENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE=Off \
    -DENABLE_BUNDLED_JSONC=On \
    -DENABLE_BUNDLED_PROTOBUF=Off \
    ${EXTRA_CMAKE_OPTIONS:-}
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Building"}
# shellcheck disable=SC2086
cmake --build "${build}" -- ${build_args}
${GITHUB_ACTIONS+echo "::endgroup::"}

if [ -t 1 ]; then
    echo
    echo "Compile with:"
    echo "cmake --build \"${build}\""
fi
