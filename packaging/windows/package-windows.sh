#!/bin/bash

repo_root="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"

if [ -z "${MSYSTEM}" ]; then
   build="${repo_root}/build-${OSTYPE}"
else
   build="${repo_root}/build-${OSTYPE}-${MSYSTEM}"
fi

if [ "$USER" = "vk" ]; then
    build="${repo_root}/build"
fi

set -exu -o pipefail

ninja -v -C "${build}" install

if [ ! -f "/msys2-installer.exe" ]; then
   wget -O /msys2-installer.exe \
      "https://github.com/msys2/msys2-installer/releases/download/2024-05-07/msys2-x86_64-20240507.exe"
fi

NDVERSION=$"$(grep 'CMAKE_PROJECT_VERSION:STATIC' "${build}/CMakeCache.txt"| cut -d= -f2)"
NDMAJORVERSION=$"$(grep 'CMAKE_PROJECT_VERSION_MAJOR:STATIC' "${build}/CMakeCache.txt"| cut -d= -f2)"
NDMINORVERSION=$"$(grep 'CMAKE_PROJECT_VERSION_MINOR:STATIC' "${build}/CMakeCache.txt"| cut -d= -f2)"

/mingw64/bin/makensis -DCURRVERSION="${NDVERSION}" -DMAJORVERSION="${NDMAJORVERSION}" -DMINORVERSION="${NDMINORVERSION}" "${repo_root}/packaging/utils/installer.nsi"
