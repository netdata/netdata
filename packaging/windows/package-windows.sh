#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
# shellcheck source=./win-build-dir.sh
source "${repo_root}/packaging/windows/win-build-dir.sh"

if [[ "${MSYSTEM:-}" != "UCRT64" ]]; then
    echo "Run this script from an MSYS2 UCRT64 terminal (MSYSTEM=UCRT64)." >&2
    exit 1
fi
export PATH="/ucrt64/bin:/ucrt64/sbin:${PATH}"
export NETDATA_WINDOWS_RUNTIME_DLL_DIR="$(cygpath -aw /ucrt64/bin)"

if [[ ! -d "${build}" || ! -f "${build}/CMakeCache.txt" || ! -f "${build}/cmake_install.cmake" ]]; then
    echo "No configured CMake build found at ${build}. Run compile-on-windows.sh first." >&2
    exit 1
fi
build_root="$(cd "${build}" && pwd -P)"
repo_root="$(cd "${repo_root}" && pwd -P)"
if [[ "${build_root}" == "${repo_root}" || ! -f "${build_root}/CMakeCache.txt" ]]; then
    echo "Refusing to clean install stage outside a configured build directory: ${build_root}" >&2
    exit 1
fi
stage_dir="${build_root}/stage/opt/netdata"
case "${stage_dir}/" in
    "${build_root}/"*) ;;
    *) echo "Refusing to clean install stage outside build directory: ${stage_dir}" >&2; exit 1 ;;
esac

if [[ -e "${stage_dir}" ]]; then
    printf 'Recreating generated install stage: %s\n' "${stage_dir}"
    rm -rf -- "${stage_dir}"
fi
mkdir -p "${stage_dir}"
/ucrt64/bin/cmake.exe --install "$(cygpath -aw "${build_root}")"

package_root="${stage_dir}"
copy_helper="${package_root}/usr/libexec/netdata/copy_files.ps1"
if [[ ! -d "$(dirname "${copy_helper}")" ]]; then
    echo "Install tree is missing the Netdata libexec directory." >&2
    exit 1
fi
cp -f "${repo_root}/packaging/windows/copy_files.ps1" "${copy_helper}"

legacy_runtime_paths="$(find "${package_root}" -type f \( -iname msys2.exe -o -iname bash.exe -o -iname sh.exe -o -iname msys-2.0.dll -o -iname mintty.exe \) -print)"
if [[ -d "${package_root}/msys64" ]]; then
    legacy_runtime_paths+="${legacy_runtime_paths:+$'\n'}${package_root}/msys64"
fi
if [[ -n "${legacy_runtime_paths}" ]]; then
    echo "Staging tree contains legacy MSYS2 runtime files:" >&2
    printf '%s\n' "${legacy_runtime_paths}" >&2
    exit 1
fi
echo "Install tree staged at ${package_root}"
