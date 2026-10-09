#!/usr/bin/env bash

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)"
# shellcheck source=./win-build-dir.sh
source "${repo_root}/packaging/windows/win-build-dir.sh"

usage() {
    cat <<EOF
Usage: $0 [--windows-path-prefix <path>] [-- <extra CMake options>]

Configure and build Netdata with the native UCRT64 toolchain from an MSYS2 UCRT64 terminal.
EOF
}

windows_path_prefix=""
extra_cmake_options=()
while (($#)); do
    case "$1" in
        --windows-path-prefix)
            if (($# < 2)); then
                echo "Missing value for --windows-path-prefix" >&2
                exit 2
            fi
            windows_path_prefix="$2"
            shift 2
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        --)
            shift
            extra_cmake_options=("$@")
            break
            ;;
        *)
            echo "Unknown argument: $1" >&2
            usage >&2
            exit 2
            ;;
    esac
done

if [[ "${MSYSTEM:-}" != "UCRT64" ]]; then
    echo "Run this script from an MSYS2 UCRT64 terminal (MSYSTEM=UCRT64)." >&2
    exit 1
fi

export PATH="/ucrt64/bin:/ucrt64/sbin:${PATH}"
pkg_config_paths="$(cygpath -aw /ucrt64/lib/pkgconfig);$(cygpath -aw /ucrt64/share/pkgconfig)"
export PKG_CONFIG_PATH="${pkg_config_paths}"
export PKG_CONFIG_LIBDIR="${PKG_CONFIG_PATH}"
unset CC CXX

for tool in cmake.exe ninja.exe gcc.exe g++.exe go.exe rustc.exe ld.lld.exe; do
    if [[ ! -x "/ucrt64/bin/${tool}" ]]; then
        echo "Required UCRT64 tool is missing: /ucrt64/bin/${tool}" >&2
        exit 1
    fi
done
if [[ ! -d /ucrt64/lib/go ]]; then
    echo "UCRT64 Go root is missing: /ucrt64/lib/go" >&2
    exit 1
fi
export GOROOT="$(cygpath -w /ucrt64/lib/go)"

# Native builds also need the Windows SDK resource/message compilers and the VS linker.
program_files_x86="$(printenv 'ProgramFiles(x86)' || true)"
if [[ -z "${program_files_x86}" ]]; then
    program_files_x86="C:/Program Files (x86)"
fi
program_files_x86="$(cygpath -u "${program_files_x86}")"
sdk_root="${program_files_x86}/Windows Kits/10/bin"
sdk_tools="$(printf '%s\n' "${sdk_root}"/*/x64 | sort -V | tail -n 1)"
if [[ ! -x "${sdk_tools}/mc.exe" || ! -x "${sdk_tools}/rc.exe" ]]; then
    echo "Windows SDK mc.exe and rc.exe were not found under ${sdk_root}." >&2
    exit 1
fi

vswhere="${program_files_x86}/Microsoft Visual Studio/Installer/vswhere.exe"
if [[ ! -x "${vswhere}" ]]; then
    echo "Visual Studio vswhere.exe was not found: ${vswhere}" >&2
    exit 1
fi
vs_root="$("${vswhere}" -latest -products '*' -requires Microsoft.VisualStudio.Component.VC.Tools.x86.x64 -property installationPath | tr -d '\r')"
if [[ -z "${vs_root}" ]]; then
    echo "Visual Studio C++ x64 build tools were not found." >&2
    exit 1
fi
vs_tools_root="$(cygpath -u "${vs_root}")/VC/Tools/MSVC"
link_tools="$(printf '%s\n' "${vs_tools_root}"/*/bin/Hostx64/x64 | sort -V | tail -n 1)"
if [[ ! -x "${link_tools}/link.exe" ]]; then
    echo "Visual Studio link.exe was not found under ${vs_tools_root}." >&2
    exit 1
fi
export PATH="${sdk_tools}:${link_tools}:${PATH}"

build_type="${CMAKE_BUILD_TYPE:-RelWithDebInfo}"
build_cflags="-Wa,-mbig-obj -pipe -D_FILE_OFFSET_BITS=64"
if [[ "${build_type}" == "Debug" ]]; then
    build_cflags="-O0 -ggdb -Wall -Wextra -Wno-char-subscripts -DNETDATA_INTERNAL_CHECKS=1 ${build_cflags} ${CFLAGS:-}"
else
    build_cflags="-O2 ${build_cflags} ${CFLAGS:-}"
fi

mkdir -p "${build}"
build_win="$(cygpath -aw "${build}")"
repo_win="$(cygpath -aw "${repo_root}")"
prefix_win="$(cygpath -aw /ucrt64)"
install_prefix_win="$(cygpath -aw "${build}/stage/opt/netdata")"
user_name="${USERNAME:-${USER:-netdata}}"
cmake_args=(
    -S "${repo_win}" -B "${build_win}" -G Ninja
    "-DCMAKE_MAKE_PROGRAM=$(cygpath -aw /ucrt64/bin/ninja.exe)"
    "-DCMAKE_C_COMPILER=$(cygpath -aw /ucrt64/bin/gcc.exe)"
    "-DCMAKE_CXX_COMPILER=$(cygpath -aw /ucrt64/bin/g++.exe)"
    "-DCMAKE_BUILD_TYPE=${build_type}"
    "-DCMAKE_PREFIX_PATH:PATH=${prefix_win}"
    "-DCMAKE_INSTALL_PREFIX=${install_prefix_win}"
    -DNETDATA_PACKAGE_KIND=msi
    "-DNETDATA_USER=${user_name}"
    -DENABLE_ACLK=On
    -DENABLE_CLOUD=On
    -DENABLE_ML=On
    -DENABLE_PLUGIN_GO=On
    -DENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE=Off
    -DENABLE_PLUGIN_SYSTEMD_JOURNAL=Off
    -DENABLE_BUNDLED_JSONC=On
    -DENABLE_BUNDLED_PROTOBUF=On
    "-DRust_COMPILER=$(cygpath -aw /ucrt64/bin/rustc.exe)"
    -DCMAKE_NINJA_FORCE_RESPONSE_FILE=ON
    '-DCMAKE_C_FLAGS_RELWITHDEBINFO=-O2 -g1 -DNDEBUG'
    '-DCMAKE_CXX_FLAGS_RELWITHDEBINFO=-O2 -g1 -DNDEBUG'
    -DCMAKE_EXE_LINKER_FLAGS=-fuse-ld=lld
    -DCMAKE_SHARED_LINKER_FLAGS=-fuse-ld=lld
)
if [[ -n "${windows_path_prefix}" ]]; then
    cmake_args+=("-DNETDATA_WINDOWS_PATH_PREFIX=${windows_path_prefix}")
fi
if ((${#extra_cmake_options[@]})); then
    cmake_args+=("${extra_cmake_options[@]}")
fi

CFLAGS="${build_cflags}" /ucrt64/bin/cmake.exe "${cmake_args[@]}"
/ucrt64/bin/cmake.exe --build "${build_win}" -- -k 1
if [[ ! -x "${build}/netdata.exe" ]]; then
    echo "Build returned success but ${build}/netdata.exe was not produced." >&2
    exit 1
fi
echo "Build complete: ${build}/netdata.exe"
