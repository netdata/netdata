#!/bin/bash

REPO_ROOT="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"

# shellcheck source=./win-build-dir.sh
. "${REPO_ROOT}/packaging/windows/win-build-dir.sh"

set -exu -o pipefail

${GITHUB_ACTIONS+echo "::group::Installing"}
cmake --install "${build}"
${GITHUB_ACTIONS+echo "::endgroup::"}

if [ ! -f "/msys2-installer.exe" ]; then
    ${GITHUB_ACTIONS+echo "::group::Fetching MSYS2 installer"}
    "${REPO_ROOT}/packaging/windows/fetch-msys2-installer.py" /msys2-installer.exe
    ${GITHUB_ACTIONS+echo "::endgroup::"}
fi

${GITHUB_ACTIONS+echo "::group::Packaging"}
NDVERSION=$"$(grep 'CMAKE_PROJECT_VERSION:STATIC' "${build}/CMakeCache.txt"| cut -d= -f2)"
NDMAJORVERSION=$"$(grep 'CMAKE_PROJECT_VERSION_MAJOR:STATIC' "${build}/CMakeCache.txt"| cut -d= -f2)"
NDMINORVERSION=$"$(grep 'CMAKE_PROJECT_VERSION_MINOR:STATIC' "${build}/CMakeCache.txt"| cut -d= -f2)"

/mingw64/bin/makensis.exe -DCURRVERSION="${NDVERSION}" -DMAJORVERSION="${NDMAJORVERSION}" -DMINORVERSION="${NDMINORVERSION}" "${REPO_ROOT}/packaging/windows/installer.nsi"
${GITHUB_ACTIONS+echo "::endgroup::"}
