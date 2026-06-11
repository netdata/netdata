#!/bin/bash

REPO_ROOT="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"
CMAKE_BUILD_TYPE="${CMAKE_BUILD_TYPE:-RelWithDebInfo}"

if [ "${MSYSTEM:-}" != "UCRT64" ]; then
    export MSYSTEM="UCRT64"
    echo "Setting MSYSTEM=UCRT64 for the Windows build." >&2
fi

# shellcheck source=./win-build-dir.sh
. "${REPO_ROOT}/packaging/windows/win-build-dir.sh"

set -eu -o pipefail

windows_path_prefix=
windows_path_prefix_arg=()

usage() {
    cat <<EOF
Usage: $0 [options]

Options:
  --windows-path-prefix <path>  Override NETDATA_WINDOWS_PATH_PREFIX for this build.
  --help                        Show this help message.
EOF
}

while [ $# -gt 0 ]; do
    case "$1" in
        --windows-path-prefix)
            if [ $# -lt 2 ]; then
                echo "Missing value for --windows-path-prefix" >&2
                exit 1
            fi
            windows_path_prefix="$2"
            shift 2
            ;;
        --help|-h)
            usage
            exit 0
            ;;
        *)
            echo "Unrecognized option '$1'" >&2
            usage >&2
            exit 1
            ;;
    esac
done

if [ -n "${windows_path_prefix}" ]; then
    windows_path_prefix_arg=("-DNETDATA_WINDOWS_PATH_PREFIX=${windows_path_prefix}")
fi

# Prepend UCRT64 toolchain so cmake finds native Windows tools before MSYS2 wrappers.
# Without this, cmake picks /usr/bin/cc (MSYS2 stub) instead of /ucrt64/bin/gcc.
# UCRT64 cmake is a native Windows binary: it converts PATH to Windows format for lookups,
# so prepending /ucrt64/bin here ensures it finds gcc and ninja from the UCRT64 toolchain.
export PATH="/ucrt64/bin:/ucrt64/sbin:${PATH}"
unset CC CXX

# Restrict pkg-config to UCRT64 paths only. Without this, pkg-config may find MSYS
# subsystem packages (e.g. msys/libuv-devel) whose .pc files add -isystem /usr/include,
# injecting POSIX/Cygwin headers into the UCRT64 compile and causing type conflicts.
export PKG_CONFIG_PATH="/ucrt64/lib/pkgconfig:/ucrt64/share/pkgconfig"
export PKG_CONFIG_LIBDIR="/ucrt64/lib/pkgconfig:/ucrt64/share/pkgconfig"

if [ -d "${build}" ]; then
	rm -rf "${build}"
fi

generator="Unix Makefiles"
build_args="-j $(nproc)"
cmake_make_program=()

# UCRT64 ninja (native Windows binary) uses CreateProcess to invoke the compiler,
# whereas MSYS2 ninja uses /bin/sh which strips backslashes from Windows-style paths,
# causing "command not found" failures at exit code 127.
if [ -x "/ucrt64/bin/ninja" ]; then
    generator="Ninja"
    build_args="-k 1"
    cmake_make_program=("-DCMAKE_MAKE_PROGRAM=/ucrt64/bin/ninja")
fi

COMMON_CFLAGS="-Wa,-mbig-obj -pipe -D_FILE_OFFSET_BITS=64 -D__USE_MINGW_ANSI_STDIO=1"

# GNU BFD ld.exe hangs (or OOMs) on large RelWithDebInfo builds because it
# cannot handle the combined DWARF load from 700+ objects + absl + protobuf.
# lld handles this correctly and links ~10x faster. lld is a hard
# dependency for the link of netdata.exe; install it via
# packaging/windows/msys2-dependencies.sh (which pulls
# mingw-w64-ucrt-x86_64-lld).
linker_cmake_flags=()
if [ -x "/ucrt64/bin/ld.lld" ]; then
    linker_cmake_flags=("-DCMAKE_EXE_LINKER_FLAGS=-fuse-ld=lld"
                        "-DCMAKE_SHARED_LINKER_FLAGS=-fuse-ld=lld")
else
    echo "WARNING: /ucrt64/bin/ld.lld not found." >&2
    echo "  The link of netdata.exe with the default BFD ld.exe is known to" >&2
    echo "  hang or run out of memory on RelWithDebInfo builds." >&2
    echo "  Install lld and re-run: pacman -S mingw-w64-ucrt-x86_64-lld" >&2
    echo "  Falling back to BFD with -g1 and --no-keep-memory mitigations." >&2
    # BFD fallback: reduce DWARF from level 2 (-g) to level 1 (-g1) so the
    # linker's memory footprint stays within bounds, and tell BFD to trade
    # speed for lower memory via --no-keep-memory.
    # FLAGS variables are space-separated strings, not CMake lists — no semicolons.
    linker_cmake_flags=(
        "-DCMAKE_C_FLAGS_RELWITHDEBINFO=-O2 -g1 -DNDEBUG"
        "-DCMAKE_CXX_FLAGS_RELWITHDEBINFO=-O2 -g1 -DNDEBUG"
        "-DCMAKE_EXE_LINKER_FLAGS=-Wl,--no-keep-memory"
    )
fi

if [ "${CMAKE_BUILD_TYPE}" = "Debug" ]; then
    BUILD_CFLAGS="-O0 -ggdb -Wall -Wextra -Wno-char-subscripts -DNETDATA_INTERNAL_CHECKS=1 ${COMMON_CFLAGS} ${CFLAGS:-}"
else
    BUILD_CFLAGS="-O2 ${COMMON_CFLAGS} ${CFLAGS:-}"
fi

${GITHUB_ACTIONS+echo "::group::Configuring"}
# shellcheck disable=SC2086
CFLAGS="${BUILD_CFLAGS}" /ucrt64/bin/cmake \
    -S "${REPO_ROOT}" \
    -B "${build}" \
    -G "${generator}" \
    "${cmake_make_program[@]}" \
    "${linker_cmake_flags[@]}" \
    -DCMAKE_BUILD_TYPE="${CMAKE_BUILD_TYPE}" \
    -DCMAKE_INSTALL_PREFIX="/opt/netdata" \
    -DBUILD_FOR_PACKAGING=On \
    -DNETDATA_USER="${USER}" \
    -DENABLE_ACLK=On \
    -DENABLE_CLOUD=On \
    -DENABLE_ML=On \
    -DENABLE_PLUGIN_GO=On \
    -DENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE=Off \
    -DENABLE_PLUGIN_OTEL=Off \
    -DENABLE_PLUGIN_SYSTEMD_JOURNAL=Off \
    -DENABLE_BUNDLED_JSONC=On \
    -DENABLE_BUNDLED_PROTOBUF=Off \
    -DRust_COMPILER=/ucrt64/bin/rustc \
    "${windows_path_prefix_arg[@]}" \
    ${EXTRA_CMAKE_OPTIONS:-}
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Building"}
# shellcheck disable=SC2086
cmake --build "${build}" -- ${build_args}
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Netdata buildinfo"}
"${build}/netdata.exe" -W buildinfo || true
${GITHUB_ACTIONS+echo "::endgroup::"}

if [ -t 1 ]; then
    echo
    echo "Compile with:"
    echo "cmake --build \"${build}\""
fi
