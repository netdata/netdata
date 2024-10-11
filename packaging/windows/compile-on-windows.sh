#!/bin/bash

PATH="${PATH}:$(cygpath -u -a 'C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x64'):$(cygpath -u -a 'C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.41.34120\bin\Hostx64\x64')"

# Versions of MC.EXE in github CI
# C:\Program Files (x86)\Microsoft Visual Studio\Shared\NuGetPackages\microsoft.windows.sdk.buildtools\10.0.22621.3233\bin\10.0.22621.0\arm64\mc.exe
# C:\Program Files (x86)\Microsoft Visual Studio\Shared\NuGetPackages\microsoft.windows.sdk.buildtools\10.0.22621.3233\bin\10.0.22621.0\x64\mc.exe
# C:\Program Files (x86)\Microsoft Visual Studio\Shared\NuGetPackages\microsoft.windows.sdk.buildtools\10.0.22621.3233\bin\10.0.22621.0\x86\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.17763.0\arm64\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.17763.0\x64\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.17763.0\x86\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.19041.0\arm64\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.19041.0\x64\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.19041.0\x86\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.20348.0\arm64\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.20348.0\x64\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.20348.0\x86\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.22000.0\arm64\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.22000.0\x64\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.22000.0\x86\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\arm64\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x64\mc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x86\mc.exe

# Versions of RC.EXE in github CI
# C:\Program Files (x86)\Microsoft Visual Studio\Shared\NuGetPackages\microsoft.windows.sdk.buildtools\10.0.22621.3233\bin\10.0.22621.0\arm64\rc.exe
# C:\Program Files (x86)\Microsoft Visual Studio\Shared\NuGetPackages\microsoft.windows.sdk.buildtools\10.0.22621.3233\bin\10.0.22621.0\x64\rc.exe
# C:\Program Files (x86)\Microsoft Visual Studio\Shared\NuGetPackages\microsoft.windows.sdk.buildtools\10.0.22621.3233\bin\10.0.22621.0\x86\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.17763.0\arm64\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.17763.0\x64\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.17763.0\x86\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.19041.0\arm64\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.19041.0\x64\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.19041.0\x86\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.20348.0\arm64\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.20348.0\x64\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.20348.0\x86\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.22000.0\arm64\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.22000.0\x64\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.22000.0\x86\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\arm64\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x64\rc.exe
# C:\Program Files (x86)\Windows Kits\10\bin\10.0.22621.0\x86\rc.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\SDK\ScopeCppSDK\vc15\SDK\bin\rc.exe

# Versions of LINK.EXE in github CI
# C:\Program Files (x86)\Microsoft SDKs\UWPNuGetPackages\runtime.win10-arm64.microsoft.net.native.compiler\2.2.12-rel-31116-00\tools\arm64\ilc\tools\link\link.exe
# C:\Program Files (x86)\Windows Kits\10\Tools\sdv\bin\interceptor\link.exe
# C:\Program Files (x86)\Windows Kits\10\Tools\sdv\smv\bin\link.exe
# C:\Program Files\Git\usr\bin\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\SDK\ScopeCppSDK\vc15\VC\bin\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.29.30133\bin\HostX64\arm\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.29.30133\bin\HostX64\arm64\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.29.30133\bin\HostX64\x64\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.29.30133\bin\HostX64\x86\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.29.30133\bin\HostX86\arm\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.29.30133\bin\HostX86\arm64\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.29.30133\bin\HostX86\x64\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.29.30133\bin\HostX86\x86\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.41.34120\bin\Hostx64\arm\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.41.34120\bin\Hostx64\arm64\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.41.34120\bin\Hostx64\x64\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.41.34120\bin\Hostx64\x86\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.41.34120\bin\Hostx86\arm\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.41.34120\bin\Hostx86\arm64\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.41.34120\bin\Hostx86\x64\link.exe
# C:\Program Files\Microsoft Visual Studio\2022\Enterprise\VC\Tools\MSVC\14.41.34120\bin\Hostx86\x86\link.exe

REPO_ROOT="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"
CMAKE_BUILD_TYPE="${CMAKE_BUILD_TYPE:-RelWithDebInfo}"

# shellcheck source=./win-build-dir.sh
. "${REPO_ROOT}/packaging/windows/win-build-dir.sh"

set -exu -o pipefail

if [ -d "${build}" ]; then
	rm -rf "${build}"
fi

generator="Unix Makefiles"
build_args="-j $(nproc)"

if command -v ninja >/dev/null 2>&1; then
    generator="Ninja"
    build_args="-k 1"
fi

COMMON_CFLAGS="-Wa,-mbig-obj -pipe -D_FILE_OFFSET_BITS=64 -D__USE_MINGW_ANSI_STDIO=1"

if [ "${CMAKE_BUILD_TYPE}" = "Debug" ]; then
    BUILD_CFLAGS="-fstack-protector-all -O0 -ggdb -Wall -Wextra -Wno-char-subscripts -DNETDATA_INTERNAL_CHECKS=1 ${COMMON_CFLAGS} ${CFLAGS:-}"
else
    BUILD_CFLAGS="-O2 ${COMMON_CFLAGS} ${CFLAGS:-}"
fi

${GITHUB_ACTIONS+echo "::group::Configuring"}
# shellcheck disable=SC2086
CFLAGS="${BUILD_CFLAGS}" /usr/bin/cmake \
    -S "${REPO_ROOT}" \
    -B "${build}" \
    -G "${generator}" \
    -DCMAKE_INSTALL_PREFIX="/opt/netdata" \
    -DBUILD_FOR_PACKAGING=On \
    -DNETDATA_USER="${USER}" \
    -DENABLE_ACLK=On \
    -DENABLE_CLOUD=On \
    -DENABLE_H2O=Off \
    -DENABLE_ML=On \
    -DENABLE_PLUGIN_GO=On \
    -DENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE=Off \
    -DENABLE_BUNDLED_JSONC=On \
    -DENABLE_BUNDLED_PROTOBUF=Off \
    ${EXTRA_CMAKE_OPTIONS:-}
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Building"}
# shellcheck disable=SC2086
cmake --build "${build}" -- ${build_args}
${GITHUB_ACTIONS+echo "::endgroup::"}

if [ -t 1 ]; then
    echo
    echo "Compile with:"
    echo "cmake --build \"${build}\""
fi
