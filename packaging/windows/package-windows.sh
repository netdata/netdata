#!/bin/bash

repo_root="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"

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

${GITHUB_ACTIONS+echo "::group::Installing"}
cmake --install "${build}"
${GITHUB_ACTIONS+echo "::endgroup::"}

if [ ! -f "/msys2-installer.exe" ]; then
    ${GITHUB_ACTIONS+echo "::group::Fetching MSYS2 installer"}
    "${repo_root}/packaging/windows/fetch-msys2-installer.py" /msys2-installer.exe
    ${GITHUB_ACTIONS+echo "::endgroup::"}
fi

${GITHUB_ACTIONS+echo "::group::Licenses"}
if [ ! -f "/gpl-3.0.txt" ]; then
    curl -o /gpl-3.0.txt "https://www.gnu.org/licenses/gpl-3.0.txt"
fi

if [ ! -f "/cloud.txt" ]; then
    curl -o /cloud.txt "https://raw.githubusercontent.com/netdata/netdata/master/src/web/gui/v2/LICENSE.md"
fi
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Prompt"}
cp /msys2.exe /msys2.ini /msys2_shell.cmd /opt/netdata
cd /usr/bin && cp ls.exe nano.exe msys-magic-1.dll msys-bz2-1.dll msys-lzma-5.dll /opt/netdata/usr/bin
echo "export PATH=\"/usr/bin:/bin:/\"" > /opt/netdata/etc/profile
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Packaging"}
NDVERSION=$"$(grep 'CMAKE_PROJECT_VERSION:STATIC' "${build}/CMakeCache.txt"| cut -d= -f2)"
NDMAJORVERSION=$"$(grep 'CMAKE_PROJECT_VERSION_MAJOR:STATIC' "${build}/CMakeCache.txt"| cut -d= -f2)"
NDMINORVERSION=$"$(grep 'CMAKE_PROJECT_VERSION_MINOR:STATIC' "${build}/CMakeCache.txt"| cut -d= -f2)"

/mingw64/bin/makensis.exe -DCURRVERSION="${NDVERSION}" -DMAJORVERSION="${NDMAJORVERSION}" -DMINORVERSION="${NDMINORVERSION}" "${repo_root}/packaging/windows/installer.nsi"
${GITHUB_ACTIONS+echo "::endgroup::"}

