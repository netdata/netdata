#!/usr/bin/env bash

set -eu -o pipefail

OPTIONS="asori"
LONGOPTIONS="address-sanitizer,sentry,optimized,root,install"

PARSED=$(getopt --options=$OPTIONS --longoptions=$LONGOPTIONS --name "$0" -- "$@")
if [[ $? -ne 0 ]]; then
    exit 2
fi

eval set -- "$PARSED"

# Initialize ASAN_OPTION variable
ASAN_OPTION=""
SENTRY_OPTION=""
BUILD_TYPE="Debug"
WT_PREFIX=""
INSTALL=""

# Extract options and arguments
while true; do
    case "$1" in
        -a|--address-sanitizer)
            ASAN_OPTION="-DENABLE_ADDRESS_SANITIZER=On"
            shift
            ;;
        -o|--optimized)
            BUILD_TYPE="RelWithDebInfo"
            shift
            ;;
        -s|--sentry)
            SENTRY_OPTION="true"
            shift
            ;;
        -r|--root)
            WT_PREFIX="/"
            shift
            ;;
        -i|--install)
            INSTALL="yes"
            shift
            ;;
        --)
            shift
            break
            ;;
        *)
            echo "Programming error"
            exit 3
            ;;
    esac
done

if [ "$#" -ne 1 ]; then
    echo "Usage: $0 [-a|--asan] [-i|--install] [-o|--optimized] [-r|--root] [-s|--sentry] <worktree-name>"
    exit 1
fi

WT_NAME="$1"
WT_ROOT="${HOME}/repos/nd/${WT_NAME}"
WT_PREFIX=${WT_PREFIX:-"${HOME}/opt/${WT_NAME}/netdata"}

pushd "${WT_ROOT}"

set -x

SENTRY_FLAGS=""
if [[ "$SENTRY_OPTION" == "true" ]]; then
    declare -A sentry_flags=(
        [ENABLE_SENTRY]="On"
        [NETDATA_SENTRY_ENVIRONMENT]="Development-CI"
        [NETDATA_SENTRY_RELEASE]="7.7.7"
        [NETDATA_SENTRY_DIST]="dist"
        [NETDATA_SENTRY_DSN]="https://4c37747b97164e9bbfc9fa426e9200b4@o382276.ingest.sentry.io/4505069981401088"
        [ENABLE_LIBBACKTRACE]="On"
    )

    for key in "${!sentry_flags[@]}"; do
        SENTRY_FLAGS+="-D${key}=${sentry_flags[$key]} "
    done
fi

/usr/bin/cmake -S "${WT_ROOT}" -B "${WT_ROOT}/build" \
    -G Ninja \
    -DCMAKE_INSTALL_PREFIX="${WT_PREFIX}" \
    -DCMAKE_BUILD_TYPE="${BUILD_TYPE}" \
    -DCMAKE_C_FLAGS="-Wall -Wextra" \
    -DCMAKE_EXE_LINKER_FLAGS="-fuse-ld=mold" \
    -DNETDATA_USER=netdata \
    -DENABLE_PLUGIN_EBPF=Off \
    -DENABLE_PLUGIN_LOGS_MANAGEMENT=On \
    -DENABLE_LOGS_MANAGEMENT_TESTS=Off \
    -DENABLE_BUNDLED_JSONC=Off \
    -DENABLE_BUNDLED_YAML=Off \
    -DENABLE_BUNDLED_PROTOBUF=Off \
    -DWEB_DIR=/var/lib/netdata/www \
    ${ASAN_OPTION} \
    ${SENTRY_FLAGS}

if [ "$INSTALL" = "yes" ]; then
    ninja -v -C "${WT_ROOT}/build" install
else
    ninja -v -C "${WT_ROOT}/build" all
fi

set +x

popd
