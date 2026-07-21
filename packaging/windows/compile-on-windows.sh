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

COMMON_CFLAGS="-Wa,-mbig-obj -pipe -D_FILE_OFFSET_BITS=64"

# GNU BFD ld.exe hangs (or OOMs) on large RelWithDebInfo builds because it
# cannot handle the combined DWARF load from 700+ objects + absl + protobuf.
# lld handles it much better, but full DWARF-2 (-g) of 424 TUs + libdlib.a
# (45 MB of absl/dlib C++ templates) + protobuf static initializers is still
# the heaviest single input to the linker on this build. Without a DWARF
# reduction, the netdata.exe link can run for hours on a 16 GB box because
# lld's PE/COFF backend merges debug sections serially and peaks at very
# high RSS. Both the BFD fallback (further down) and the lld branch reduce
# the debug level to -g1 so the linker has something tractable to chew on.
# -g1 keeps function names + line numbers (sufficient for crash stacks);
# only local-variable and macro debug info is lost.
#
# How lld is selected: UCRT64 GCC is a DRIVER-mode compiler in CMake terms
# (CMAKE_<LANG>_LINK_MODE=DRIVER), so the link rule is the compiler driver,
# not ${CMAKE_LINKER}. The driver is told to re-route the link to lld via
# `-fuse-ld=lld`; the driver then finds `ld.lld` in PATH (line 59 above
# already prepends /ucrt64/bin). CMake docs document this exact path:
# https://cmake.org/cmake/help/latest/variable/CMAKE_LANG_USING_LINKER_TYPE.html
# (CMAKE_LINKER_TYPE=LLD, CMake 3.29+, expands to the same `-fuse-ld=lld`
# for DRIVER mode; project's stated cmake_minimum_required is 3.16, so we
# use the portable form directly.)
#
# Do NOT set -DCMAKE_LINKER=/path/to/ld.lld here: in DRIVER mode CMake does
# not substitute ${CMAKE_LINKER} into the default link rule, so it has no
# effect on which linker actually runs.
#
# Do NOT pass -Wl,--no-keep-memory in the lld branch: that flag is a GNU
# BFD memory mitigation (re-read symbol tables instead of caching). lld has
# its own design (mmap + parallel hashmaps) and rejects the flag with
# "lld: error: unknown argument: --no-keep-memory", failing CMake's
# initial compiler-test link. The flag is kept only in the BFD fallback.
linker_cmake_flags=()
if [ -x "/ucrt64/bin/ld.lld" ]; then
    linker_cmake_flags=("-DCMAKE_C_FLAGS_RELWITHDEBINFO=-O2 -g1 -DNDEBUG"
                        "-DCMAKE_CXX_FLAGS_RELWITHDEBINFO=-O2 -g1 -DNDEBUG"
                        "-DCMAKE_EXE_LINKER_FLAGS=-fuse-ld=lld"
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
    -DCMAKE_PREFIX_PATH=/ucrt64 \
    -DCMAKE_INSTALL_PREFIX="/opt/netdata" \
    -DBUILD_FOR_PACKAGING=On \
    -DNETDATA_USER="${USER}" \
    -DENABLE_ACLK=On \
    -DENABLE_CLOUD=On \
    -DENABLE_ML=On \
    -DENABLE_PLUGIN_GO=On \
    -DENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE=Off \
    -DENABLE_PLUGIN_SYSTEMD_JOURNAL=Off \
    -DENABLE_BUNDLED_JSONC=On \
    -DENABLE_BUNDLED_PROTOBUF=On \
    -DRust_COMPILER=/ucrt64/bin/rustc.exe \
    -DCMAKE_NINJA_FORCE_RESPONSE_FILE=ON \
    "${windows_path_prefix_arg[@]}" \
    ${EXTRA_CMAKE_OPTIONS:-}

# Why -DCMAKE_NINJA_FORCE_RESPONSE_FILE=ON:
#   The netdata.exe link pulls in 424 .obj files + ~30 archives + system
#   libs. Without response files the full ~28 KB argument list is inlined,
#   which exceeds cmd.exe's 8 KB command-line limit and ninja then prints
#   "The command line is too long." and the link aborts (no exe, no .dll.a;
#   the "missing exe" guard catches it).
#
#   The CORRECT variable name is CMAKE_NINJA_FORCE_RESPONSE_FILE (CMake
#   3.4+). Earlier iterations used CMAKE_NINJA_USE_RESPONSE_FILE and
#   _FOR_OBJECTS / _FOR_LIBRARIES; those names do NOT exist in CMake and
#   were silently ignored (the configure log warned "Manually-specified
#   variables were not used by the project"). CMake's Windows-GNU platform
#   module already sets __WINDOWS_GNU_LD_RESPONSE=1 so response files are
#   normally generated automatically, but FORCE_RESPONSE_FILE makes the
#   behaviour explicit and uniform across compile rules too.
#
#   No-op on non-Ninja generators and on non-Windows (this script only runs
#   on Windows). Diagnostic: SOW P22/P26 (cmd.exe 8 KB command-line limit).
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Building"}
# Capture cmake --build's exit code. Without `|| build_rc=$?`, `set -e` (set
# at line 14) would exit the script at the failing command and the
# diagnostic block below would never run, leaving the user with no
# actionable error message in stderr.
# shellcheck disable=SC2086
build_rc=0
cmake --build "${build}" -- ${build_args} || build_rc=$?
${GITHUB_ACTIONS+echo "::endgroup::"}

if [ "${build_rc}" -ne 0 ]; then
    echo "ERROR: cmake --build exited with ${build_rc}." >&2
    echo "  The most common cause on Windows UCRT64 is the netdata.exe link" >&2
    echo "  being killed (OOM, session timeout) or BFD ld.exe hanging." >&2
    echo "  Ensure mingw-w64-ucrt-x86_64-lld is installed:" >&2
    echo "    pacman -S mingw-w64-ucrt-x86_64-lld" >&2
    exit "${build_rc}"
fi

# Verify the netdata.exe artifact actually exists. cmake --build can return
# 0 even when the linker was killed (e.g. by Windows OOM or a session timer)
# because ninja's child-process exit mapping is implementation-defined.
# This guard makes a silent-kill failure impossible to mistake for success.
if [ ! -x "${build}/netdata.exe" ]; then
    echo "ERROR: cmake --build returned 0 but ${build}/netdata.exe is missing." >&2
    echo "  The link of netdata.exe was likely killed (OOM, session timeout)" >&2
    echo "  or hung. See the last lines of the build output above for the" >&2
    echo "  point at which it stopped." >&2
    exit 1
fi

echo ""
echo "============================================================"
echo "BUILD COMPLETE: ${build}/netdata.exe ($(stat -c %s "${build}/netdata.exe" 2>/dev/null || echo '?') bytes)"
echo "============================================================"
echo ""

${GITHUB_ACTIONS+echo "::group::Staging Windows runtime DLLs"}
"${REPO_ROOT}/packaging/windows/stage-runtime-dlls.sh" "${build}/netdata.exe" "${build}"
${GITHUB_ACTIONS+echo "::endgroup::"}

${GITHUB_ACTIONS+echo "::group::Netdata buildinfo"}
buildinfo_out="${build}/netdata-buildinfo.out"
buildinfo_err="${build}/netdata-buildinfo.err"
rm -f "${buildinfo_out}" "${buildinfo_err}"

# Run the native Windows binary under PowerShell instead of MSYS2 timeout. MSYS2
# timeout has been observed to leave native Windows children running, which makes
# the compile wrapper appear unfinished even after netdata.exe has linked.
if command -v powershell.exe >/dev/null 2>&1; then
    buildinfo_exe="$(cygpath -w "${build}/netdata.exe")"
    buildinfo_out_win="$(cygpath -w "${buildinfo_out}")"
    buildinfo_err_win="$(cygpath -w "${buildinfo_err}")"
    buildinfo_ucrt64_bin="$(cygpath -w /ucrt64/bin)"
    buildinfo_ucrt64_sbin="$(cygpath -w /ucrt64/sbin)"
    buildinfo_usr_bin="$(cygpath -w /usr/bin)"

    powershell.exe -NoProfile -ExecutionPolicy Bypass -Command "\
\$env:MSYSTEM = 'UCRT64'; \
\$env:PATH = '${buildinfo_ucrt64_bin};${buildinfo_ucrt64_sbin};${buildinfo_usr_bin};' + \$env:PATH; \
\$p = Start-Process -FilePath '${buildinfo_exe}' -ArgumentList '-W','buildinfo' -NoNewWindow -RedirectStandardOutput '${buildinfo_out_win}' -RedirectStandardError '${buildinfo_err_win}' -PassThru; \
if (-not \$p.WaitForExit(60 * 1000)) { \
    Stop-Process -Id \$p.Id -Force; \
    exit 124; \
}; \
exit \$p.ExitCode" || buildinfo_rc=$?
else
    timeout 60 "${build}/netdata.exe" -W buildinfo > "${buildinfo_out}" 2> "${buildinfo_err}" || buildinfo_rc=$?
fi

cat "${buildinfo_out}" 2>/dev/null || true
cat "${buildinfo_err}" >&2 2>/dev/null || true

if [ "${buildinfo_rc:-0}" -eq 124 ]; then
    echo "WARNING: 'netdata.exe -W buildinfo' did not return within 60 s." >&2
    echo "  The build itself is successful - the binary at ${build}/netdata.exe is usable." >&2
elif [ "${buildinfo_rc:-0}" -ne 0 ]; then
    echo "WARNING: 'netdata.exe -W buildinfo' exited ${buildinfo_rc} (non-fatal)." >&2
fi
${GITHUB_ACTIONS+echo "::endgroup::"}

if [ -t 1 ]; then
    echo
    echo "Compile with:"
    echo "cmake --build \"${build}\""
fi
