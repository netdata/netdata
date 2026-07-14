#!/bin/bash

repo_root="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"
# win-build-dir.sh references ${REPO_ROOT} (uppercase); set it here so the
# build directory resolves correctly when BUILD_DIR is not explicitly provided.
REPO_ROOT="${repo_root}"

# shellcheck source=./win-build-dir.sh
. "${repo_root}/packaging/windows/win-build-dir.sh"

# Prepend UCRT64 toolchain so cmake, ldd.exe, and other build tools resolve
# to the UCRT64 native Windows binaries instead of the POSIX MSYS2 wrappers.
# cmake --install must use the same cmake that generated cmake_install.cmake
# (i.e. /ucrt64/bin/cmake); the POSIX cmake at /usr/bin/cmake treats Windows
# absolute paths (C:/...) as relative paths and fails with path concatenation
# errors.  This mirrors the PATH setup in compile-on-windows.sh.
export PATH="/ucrt64/bin:/ucrt64/sbin:${PATH}"

set -eu -o pipefail

# Regenerate keys everytime there is an update
if [ -d /opt/netdata/etc/pki/ ]; then
    rm -rf /opt/netdata/etc/pki/
fi

# Remove previous installation of msys2 script
if [ -f /opt/netdata/usr/bin/bashbug ]; then
    rm -rf /opt/netdata/usr/bin/bashbug
fi

runtime_dll_destination="/opt/netdata/usr/bin"
mkdir -p "${runtime_dll_destination}"

${GITHUB_ACTIONS+echo "::group::Staging Windows runtime DLLs"}
# Resolve build to an absolute POSIX path so the glob expands correctly
# regardless of CWD when the script is invoked.
case "${build}" in
    /*) build_abs="${build}" ;;
    *)  build_abs="$(cd "${build}" && pwd -P)" ;;
esac

# compile-on-windows.sh stages all transitive UCRT64 runtime DLLs for
# netdata.exe into the build root via stage-runtime-dlls.sh.  Copy them to
# both destinations BEFORE cmake --install so they are present when cmake
# installs the executables alongside them.
#
# netdata.exe and other sbin executables need DLLs in usr/bin.
# C plugin executables (apps.plugin.exe, windows-events.plugin.exe, etc.)
# need DLLs co-located in plugins.d: Windows loads DLLs from the executable's
# own directory first, and the SYSTEM service account has no MSYS2/UCRT64
# bin directory in its DLL search path.
plugins_dll_destination="/opt/netdata/usr/libexec/netdata/plugins.d"
mkdir -p "${plugins_dll_destination}"

for dll in "${build_abs}"/lib*.dll "${build_abs}"/zlib1.dll; do
    [ -f "${dll}" ] || continue
    cp "${dll}" "${runtime_dll_destination}/"
    cp "${dll}" "${plugins_dll_destination}/"
done
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Installing"}
/ucrt64/bin/cmake --install "${build}"
${GITHUB_ACTIONS+echo "::endgroup::"}

if [ ! -f "/msys2-latest.tar.zst" ]; then
    ${GITHUB_ACTIONS+echo "::group::Fetching MSYS2 files"}
    "${repo_root}/packaging/windows/fetch-msys2-installer.py" /msys2-latest.tar.zst
    ${GITHUB_ACTIONS+echo "::endgroup::"}
fi

${GITHUB_ACTIONS+echo "::group::Licenses"}
if [ ! -f "/gpl-3.0.txt" ]; then
    cp "${repo_root}/LICENSE" /gpl-3.0.txt
fi

if [ ! -f "/cloud.txt" ]; then
    curl -o /cloud.txt "https://app.netdata.cloud/LICENSE.txt"
fi
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Copy Files"}
tar -xf /msys2-latest.tar.zst -C /opt/netdata/ || exit 1
cp -R /opt/netdata/msys64/* /opt/netdata/ || exit 1
cp packaging/windows/copy_files.ps1 /opt/netdata/usr/libexec/netdata/ || exit 1
rm -rf /opt/netdata/msys64/
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Configure Editor"}
if [ -f "/opt/netdata/etc/profile" ]; then
    echo 'EDITOR="/usr/bin/nano.exe"' >> /opt/netdata/etc/profile
fi
${GITHUB_ACTIONS+echo "::endgroup::"}

# TODO: We will have a PR to adjust CAB file creation and sign. This is only adding necessary structure
#${GITHUB_ACTIONS+echo "::group::CAB file"}
#mkdir "${build}/driver"
#cp "${build}/usr/bin/netdata_driver.*" "${build}/driver"
#powershell.exe -ExecutionPolicy Bypass -File "${repo_root}/packaging/windows/generate-driver-catalog.ps1"
#${GITHUB_ACTIONS+echo "::endgroup::"}
