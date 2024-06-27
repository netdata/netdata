#!/bin/sh
#
# Invoked by the package builder images to actually build native packages.

set -e

PKG_TYPE="${1}"
BUILD_DIR="${2}"
SCRIPT_SOURCE="$(
    self=${0}
    while [ -L "${self}" ]
    do
        cd "${self%/*}" || exit 1
        self=$(readlink "${self}")
    done
    cd "${self%/*}" || exit 1
    echo "$(pwd -P)/${self##*/}"
)"
SOURCE_DIR="$(dirname "$(dirname "${SCRIPT_SOURCE}")")"

CMAKE_ARGS="-S ${SOURCE_DIR} -B ${BUILD_DIR} -G Ninja"

add_cmake_option() {
    CMAKE_ARGS="${CMAKE_ARGS} -D${1}=${2}"
}

add_cmake_option CMAKE_BUILD_TYPE RelWithDebInfo
add_cmake_option CMAKE_INSTALL_PREFIX /
add_cmake_option ENABLE_ACLK On
add_cmake_option ENABLE_CLOUD On
add_cmake_option ENABLE_DBENGINE On
add_cmake_option ENABLE_H2O On
add_cmake_option ENABLE_ML On

add_cmake_option ENABLE_PLUGIN_APPS On
add_cmake_option ENABLE_PLUGIN_CGROUP_NETWORK On
add_cmake_option ENABLE_PLUGIN_DEBUGFS On
add_cmake_option ENABLE_PLUGIN_FREEIPMI On
add_cmake_option ENABLE_PLUGIN_GO On
add_cmake_option ENABLE_PLUGIN_LOCAL_LISTENERS On
add_cmake_option ENABLE_PLUGIN_LOGS_MANAGEMENT Off
add_cmake_option ENABLE_PLUGIN_NFACCT On
add_cmake_option ENABLE_PLUGIN_PERF On
add_cmake_option ENABLE_PLUGIN_SLABINFO On
add_cmake_option ENABLE_PLUGIN_SYSTEMD_JOURNAL On

add_cmake_option ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE On
add_cmake_option ENABLE_EXPORTER_MONGODB On

add_cmake_option ENABLE_BUNDLED_PROTOBUF Off
add_cmake_option ENABLE_BUNDLED_JSONC Off
add_cmake_option ENABLE_BUNDLED_YAML Off

add_cmake_option BUILD_FOR_PACKAGING On

case "${PKG_TYPE}" in
    DEB)
        case "$(dpkg-architecture -q DEB_TARGET_ARCH)" in
            amd64)
                add_cmake_option ENABLE_PLUGIN_XENSTAT On
                add_cmake_option ENABLE_PLUGIN_EBPF On
                ;;
            arm64)
                add_cmake_option ENABLE_PLUGIN_XENSTAT On
                add_cmake_option ENABLE_PLUGIN_EBPF Off
                ;;
            *)
                add_cmake_option ENABLE_PLUGIN_XENSTAT Off
                add_cmake_option ENABLE_PLUGIN_EBPF Off
                ;;
        esac
        ;;
    RPM) ;;
    *) echo "Unrecognized package type ${PKG_TYPE}." ; exit 1 ;;
esac

if [ "${ENABLE_SENTRY}" = "true" ]; then
    if [ -z "${SENTRY_DSN}" ]; then
        echo "ERROR: Sentry enabled but no DSN specified, exiting."
        exit 1
    fi

    add_cmake_option ENABLE_SENTRY On
    add_cmake_option NETDATA_SENTRY_ENVIRONMENT "${RELEASE_PIPELINE:-Unknown}"
    add_cmake_option NETDATA_SENTRY_DIST "${BUILD_DESTINATION:-Unknown}"
    add_cmake_option NETDATA_SENTRY_DSN "${SENTRY_DSN}"
else
    add_cmake_option ENABLE_SENTRY Off
fi

# shellcheck disable=SC2086
cmake ${CMAKE_ARGS}
cmake --build "${BUILD_DIR}" --parallel "$(nproc)"

if [ "${ENABLE_SENTRY}" = "true" ] && [ "${UPLOAD_SENTRY}" = "true" ]; then
    sentry-cli debug-files upload -o netdata-inc -p netdata-agent --force-foreground --log-level=debug --wait --include-sources build/netdata
fi

cd "${BUILD_DIR}" || exit 1
cpack -V -G "${PKG_TYPE}"
