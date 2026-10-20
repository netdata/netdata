#!/bin/bash

REPO_ROOT="$(dirname "$(dirname "$(cd "$(dirname "${BASH_SOURCE[0]}")" > /dev/null && pwd -P)")")"
CMAKE_BUILD_TYPE="${CMAKE_BUILD_TYPE:-RelWithDebInfo}"

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
    BUILD_CFLAGS="-O0 -ggdb -Wall -Wextra -Wno-char-subscripts -DNETDATA_INTERNAL_CHECKS=1 ${COMMON_CFLAGS} ${CFLAGS:-}"
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
