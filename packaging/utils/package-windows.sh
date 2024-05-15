#!/bin/sh

export PATH="/usr/local/bin:${PATH}"

WT_ROOT="$(pwd)"
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

ninja -v -C "${build}" install

if [ ! -f "/msys2-installer.exe" ]; then
   wget -O /msys2-installer.exe \
      "https://github.com/msys2/msys2-installer/releases/download/2024-05-07/msys2-x86_64-20240507.exe"
fi

makensis "${WT_ROOT}/packaging/utils/installer.nsi"
