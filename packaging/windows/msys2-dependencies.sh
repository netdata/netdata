#!/bin/bash
#
# Install the dependencies we need to build Netdata on MSYS2

. /etc/profile

set -euo pipefail

if [ "${MSYSTEM:-}" != "UCRT64" ]; then
    echo "Expected MSYSTEM=UCRT64 for the Windows build dependency setup." >&2
    exit 1
fi

${GITHUB_ACTIONS+echo "::group::Updating MSYS2"}
pacman -Syuu --noconfirm
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Installing dependencies"}
pacman -S --noconfirm --needed \
    base-devel \
    cmake \
    git \
    ninja \
    python \
    liblz4-devel \
    libutil-linux \
    libutil-linux-devel \
    libyaml-devel \
    libzstd-devel \
    msys2-devel \
    msys/brotli-devel \
    msys/libuv-devel \
    msys/pcre2-devel \
    msys/zlib-devel \
    msys/libcurl-devel \
    openssl-devel \
    protobuf-devel \
    mingw-w64-ucrt-x86_64-rust \
    mingw-w64-ucrt-x86_64-toolchain \
    ucrt64/mingw-w64-ucrt-x86_64-brotli \
    ucrt64/mingw-w64-ucrt-x86_64-cmake \
    ucrt64/mingw-w64-ucrt-x86_64-headers-git \
    ucrt64/mingw-w64-ucrt-x86_64-curl \
    ucrt64/mingw-w64-ucrt-x86_64-go \
    ucrt64/mingw-w64-ucrt-x86_64-libuv \
    ucrt64/mingw-w64-ucrt-x86_64-libyaml \
    ucrt64/mingw-w64-ucrt-x86_64-lz4 \
    ucrt64/mingw-w64-ucrt-x86_64-nsis \
    ucrt64/mingw-w64-ucrt-x86_64-openssl \
    ucrt64/mingw-w64-ucrt-x86_64-pcre2 \
    ucrt64/mingw-w64-ucrt-x86_64-protobuf \
    ucrt64/mingw-w64-ucrt-x86_64-zlib
${GITHUB_ACTIONS+echo "::endgroup::"}
