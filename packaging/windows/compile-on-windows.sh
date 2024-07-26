#!/bin/bash

repo_root="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"
BUILD_TYPE="Debug"

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

${GITHUB_ACTIONS+echo "::group::Configuring"}
# shellcheck disable=SC2086
/usr/bin/cmake -S "${repo_root}" -B "${build}" \
    -G "${generator}" \
    -DCMAKE_INSTALL_PREFIX="/opt/netdata" \
    -DCMAKE_BUILD_TYPE="${BUILD_TYPE}" \
    -DCMAKE_C_FLAGS="-fstack-protector-all -O0 -ggdb -Wall -Wextra -Wno-char-subscripts -Wa,-mbig-obj -pipe -DNETDATA_INTERNAL_CHECKS=1 -D_FILE_OFFSET_BITS=64 -D__USE_MINGW_ANSI_STDIO=1" \
    -DBUILD_FOR_PACKAGING=On \
    -DNETDATA_USER="${USER}" \
    -DDEFAULT_FEATURE_STATE=Off \
    -DENABLE_H2O=Off \
    -DENABLE_ACLK=On \
    -DENABLE_CLOUD=On \
    -DENABLE_ML=On \
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
