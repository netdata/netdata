#!/bin/bash

# On MSYS2, install these dependencies to build netdata:
install_dependencies() {
    pacman -S \
        git cmake ninja base-devel msys2-devel \
        libyaml-devel libzstd-devel libutil-linux libutil-linux-devel \
        mingw-w64-x86_64-toolchain mingw-w64-ucrt-x86_64-toolchain \
        mingw64/mingw-w64-x86_64-mold ucrt64/mingw-w64-ucrt-x86_64-mold \
        msys/gdb ucrt64/mingw-w64-ucrt-x86_64-gdb mingw64/mingw-w64-x86_64-gdb \
        msys/zlib-devel mingw64/mingw-w64-x86_64-zlib ucrt64/mingw-w64-ucrt-x86_64-zlib \
        msys/libuv-devel ucrt64/mingw-w64-ucrt-x86_64-libuv mingw64/mingw-w64-x86_64-libuv \
        liblz4-devel mingw64/mingw-w64-x86_64-lz4 ucrt64/mingw-w64-ucrt-x86_64-lz4 \
        openssl-devel mingw64/mingw-w64-x86_64-openssl ucrt64/mingw-w64-ucrt-x86_64-openssl \
        protobuf-devel mingw64/mingw-w64-x86_64-protobuf ucrt64/mingw-w64-ucrt-x86_64-protobuf \
        msys/pcre2-devel mingw64/mingw-w64-x86_64-pcre2 ucrt64/mingw-w64-ucrt-x86_64-pcre2 \
        msys/brotli-devel mingw64/mingw-w64-x86_64-brotli ucrt64/mingw-w64-ucrt-x86_64-brotli \
        msys/ccache ucrt64/mingw-w64-ucrt-x86_64-ccache mingw64/mingw-w64-x86_64-ccache \
        mingw64/mingw-w64-x86_64-go ucrt64/mingw-w64-ucrt-x86_64-go \
        mingw64/mingw-w64-x86_64-nsis
}

if [ "${1}" = "install" ]
then
	install_dependencies || exit 1
	exit 0
fi

BUILD_FOR_PACKAGING="Off"
if [ "${1}" = "package" ]; then
	BUILD_FOR_PACKAGING="On"
fi

repo_root="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"
BUILD_TYPE="Debug"

if [ -z "${MSYSTEM}" ]; then
   build="${repo_root}/build-${OSTYPE}"
else
   build="${repo_root}/build-${OSTYPE}-${MSYSTEM}"
fi

if [ "$USER" = "vk" ]; then
    build="${repo_root}/build"
fi

set -exu -o pipefail

if [ -d "${build}" ]; then
	rm -rf "${build}"
fi

/usr/bin/cmake -S "${repo_root}" -B "${build}" \
    -G Ninja \
    -DCMAKE_INSTALL_PREFIX="/opt/netdata" \
    -DCMAKE_BUILD_TYPE="${BUILD_TYPE}" \
    -DCMAKE_C_FLAGS="-fstack-protector-all -O0 -ggdb -Wall -Wextra -Wno-char-subscripts -Wa,-mbig-obj -pipe -DNETDATA_INTERNAL_CHECKS=1 -D_FILE_OFFSET_BITS=64 -D__USE_MINGW_ANSI_STDIO=1" \
    -DBUILD_FOR_PACKAGING=${BUILD_FOR_PACKAGING} \
    -DUSE_MOLD=Off \
    -DNETDATA_USER="${USER}" \
    -DDEFAULT_FEATURE_STATE=Off \
    -DENABLE_H2O=Off \
    -DENABLE_ACLK=On \
    -DENABLE_CLOUD=On \
    -DENABLE_ML=On \
    -DENABLE_BUNDLED_JSONC=On \
    -DENABLE_BUNDLED_PROTOBUF=Off

ninja -v -C "${build}" || ninja -v -C "${build}" -j 1

echo
echo "Compile with:"
echo "ninja -v -C \"${build}\" || ninja -v -C \"${build}\" -j 1"
