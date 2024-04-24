#!/bin/sh

set -exu -o pipefail

export PATH="/usr/local/bin:${PATH}"

WT_ROOT="$(pwd)"
WT_PREFIX="/tmp"
BUILD_TYPE="Debug"

if [ -z "${MSYSTEM}" ]; then
   build="${WT_ROOT}/build-${OSTYPE}"
else
   build="${WT_ROOT}/build-${OSTYPE}-${MSYSTEM}"
fi

if [ -d "${build}" ]
then
	rm -rf "${build}"
fi

/usr/bin/cmake -S "${WT_ROOT}" -B "${WT_ROOT}/build" \
    -G Ninja \
    -DCMAKE_INSTALL_PREFIX="${WT_PREFIX}" \
    -DCMAKE_BUILD_TYPE="${BUILD_TYPE}" \
    -DCMAKE_C_FLAGS="-O0 -ggdb -Wall -Wextra -Wno-char-subscripts -DNETDATA_INTERNAL_CHECKS=1" \
    -DNETDATA_USER="${USER}" \
    -DDEFAULT_FEATURE_STATE=Off \
    -DENABLE_H2O=Off \
    -DENABLE_LOGS_MANAGEMENT_TESTS=Off \
    -DENABLE_ACLK=On \
    -DENABLE_BUNDLED_PROTOBUF=Off \
    ${NULL}

ninja -v -C "${build}" || ninja -v -C "${build}" -j 1

echo
echo "Compile with:"
echo "ninja -v -C \"${build}\" || ninja -v -C \"${build}\" -j 1"
