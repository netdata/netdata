#!/bin/bash
#
# Install the dependencies we need to build Netdata on MSYS2

. /etc/profile

set -euo pipefail

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
    mingw-w64-x86_64-rust \
    mingw-w64-x86_64-toolchain \
    mingw-w64-ucrt-x86_64-rust \
    mingw-w64-ucrt-x86_64-toolchain \
    mingw64/mingw-w64-x86_64-brotli \
    mingw64/mingw-w64-x86_64-go \
    mingw64/mingw-w64-x86_64-libuv \
    mingw64/mingw-w64-x86_64-lz4 \
    mingw64/mingw-w64-x86_64-nsis \
    mingw64/mingw-w64-x86_64-openssl \
    mingw64/mingw-w64-x86_64-pcre2 \
    mingw64/mingw-w64-x86_64-protobuf \
    mingw64/mingw-w64-x86_64-zlib \
    ucrt64/mingw-w64-ucrt-x86_64-brotli \
    ucrt64/mingw-w64-ucrt-x86_64-go \
    ucrt64/mingw-w64-ucrt-x86_64-libuv \
    ucrt64/mingw-w64-ucrt-x86_64-lz4 \
    ucrt64/mingw-w64-ucrt-x86_64-openssl \
    ucrt64/mingw-w64-ucrt-x86_64-pcre2 \
    ucrt64/mingw-w64-ucrt-x86_64-protobuf \
    ucrt64/mingw-w64-ucrt-x86_64-zlib
${GITHUB_ACTIONS+echo "::endgroup::"}
