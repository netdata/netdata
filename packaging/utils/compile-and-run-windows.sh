#!/bin/sh

# On MSYS2, install these dependencies to build netdata:
install_dependencies() {
    pacman -S \
        git cmake ninja clang base-devel msys2-devel \
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
        mingw64/mingw-w64-x86_64-nsis \
        msys/libcurl msys/libcurl-devel
}

if [ "${1}" = "install" ]
then
	install_dependencies || exit 1
	exit 0
fi

BUILD_FOR_PACKAGING="Off"
if [ "${1}" = "package" ]
then
	BUILD_FOR_PACKAGING="On"
fi

export PATH="/usr/local/bin:${PATH}"

WT_ROOT="$(pwd)"
BUILD_TYPE="Debug"
NULL=""

if [ -z "${MSYSTEM}" ]; then
   build="${WT_ROOT}/build-${OSTYPE}"
else
   build="${WT_ROOT}/build-${OSTYPE}-${MSYSTEM}"
fi

if [ "$USER" = "vk" ]; then
    build="${WT_ROOT}/build"
fi

set -exu -o pipefail

if [ ! -d "${build}" ]
then
  /usr/bin/cmake -S "${WT_ROOT}" -B "${build}" \
      -G Ninja \
      -DCMAKE_INSTALL_PREFIX="/opt/netdata" \
      -DCMAKE_BUILD_TYPE="${BUILD_TYPE}" \
      -DCMAKE_C_FLAGS="-fstack-protector-all -O0 -ggdb -Wall -Wextra -Wno-char-subscripts -Wa,-mbig-obj -pipe -DNETDATA_INTERNAL_CHECKS=1 -D_FILE_OFFSET_BITS=64 -D__USE_MINGW_ANSI_STDIO=1" \
      -DBUILD_FOR_PACKAGING=${BUILD_FOR_PACKAGING} \
      -DUSE_MOLD=Off \
      -DNETDATA_USER="${USER}" \
      -DDEFAULT_FEATURE_STATE=Off \
      -DENABLE_H2O=Off \
      -DENABLE_ML=On \
      -DENABLE_BUNDLED_JSONC=On \
      -DENABLE_BUNDLED_PROTOBUF=Off \
      -DENABLE_PLUGIN_APPS=On \
      ${NULL}
fi

ninja -v -C "${build}" || ninja -v -C "${build}" -j 1

echo "Stopping service Netdata"
sc stop "Netdata" || echo "Failed"

ninja -v -C "${build}" install || ninja -v -C "${build}" -j 1

# register the event log publisher
cmd.exe //c "$(cygpath -w -a "/opt/netdata/usr/bin/wevt_netdata_install.bat")"

#echo
#echo "Compile with:"
#echo "ninja -v -C \"${build}\" install || ninja -v -C \"${build}\" -j 1"

echo "starting netdata..."
# enable JIT debug with gdb
export MSYS="error_start:$(cygpath -w /usr/bin/gdb)"

rm -rf /opt/netdata/var/log/netdata/*.log || echo
/opt/netdata/usr/bin/netdata -D
