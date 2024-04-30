#!/bin/sh

# On MSYS2, install these dependencies to build netdata:
install_dependencies() {
    pacman -S \
       git cmake ninja \
        base-devel msys2-devel libyaml-devel libzstd-devel libutil-linux libutil-linux-devel \
        mingw-w64-x86_64-toolchain mingw-w64-ucrt-x86_64-toolchain \
        msys/zlib-devel mingw64/mingw-w64-x86_64-zlib ucrt64/mingw-w64-ucrt-x86_64-zlib \
        msys/libuv-devel ucrt64/mingw-w64-ucrt-x86_64-libuv mingw64/mingw-w64-x86_64-libuv \
        liblz4-devel mingw64/mingw-w64-x86_64-lz4 ucrt64/mingw-w64-ucrt-x86_64-lz4 \
        openssl-devel mingw64/mingw-w64-x86_64-openssl ucrt64/mingw-w64-ucrt-x86_64-openssl \
        protobuf-devel mingw64/mingw-w64-x86_64-protobuf ucrt64/mingw-w64-ucrt-x86_64-protobuf \
        msys/pcre2-devel mingw64/mingw-w64-x86_64-pcre2 ucrt64/mingw-w64-ucrt-x86_64-pcre2 \
        msys/brotli-devel mingw64/mingw-w64-x86_64-brotli ucrt64/mingw-w64-ucrt-x86_64-brotli
}

export PATH="/usr/local/bin:${PATH}"

WT_ROOT="$(pwd)"
WT_PREFIX="/opt/netdata"
BUILD_TYPE="Debug"
NULL=""

if [ -z "${MSYSTEM}" ]; then
   build="${WT_ROOT}/build-${OSTYPE}"
else
   build="${WT_ROOT}/build-${OSTYPE}-${MSYSTEM}"
fi

set -exu -o pipefail

if [ -d "${build}" ]
then
	rm -rf "${build}"
fi

/usr/bin/cmake -S "${WT_ROOT}" -B "${build}" \
    -G Ninja \
    -DCMAKE_INSTALL_PREFIX="${WT_PREFIX}" \
    -DCMAKE_BUILD_TYPE="${BUILD_TYPE}" \
    -DCMAKE_C_FLAGS="-O0 -ggdb -Wall -Wextra -Wno-char-subscripts -Wa,-mbig-obj -DNETDATA_INTERNAL_CHECKS=1" \
    -DNETDATA_USER="${USER}" \
    -DDEFAULT_FEATURE_STATE=Off \
    -DENABLE_H2O=Off \
    -DENABLE_LOGS_MANAGEMENT_TESTS=Off \
    -DENABLE_ACLK=On \
    -DENABLE_ML=On \
    -DENABLE_BUNDLED_PROTOBUF=Off \
    ${NULL}

ninja -v -C "${build}" || ninja -v -C "${build}" -j 1

echo
echo "Compile with:"
echo "ninja -v -C \"${build}\" || ninja -v -C \"${build}\" -j 1"
