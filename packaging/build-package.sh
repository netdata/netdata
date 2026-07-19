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

. /etc/os-release

# Keep one argument per line so POSIX sh preserves embedded spaces
# without relying on arrays or eval.
CMAKE_ARGS="-S
${SOURCE_DIR}
-B
${BUILD_DIR}
-G
Ninja"

add_cmake_option() {
    CMAKE_ARGS="${CMAKE_ARGS}
-D${1}=${2}"
}

run_cmake() {
    (
        set --
        while IFS= read -r arg; do
            [ -n "${arg}" ] || continue
            set -- "$@" "${arg}"
        done <<EOF
${CMAKE_ARGS}
EOF
        cmake "$@"
    )
}

add_cmake_option CMAKE_BUILD_TYPE RelWithDebInfo
add_cmake_option CMAKE_INSTALL_PREFIX /
add_cmake_option ENABLE_DASHBOARD on
add_cmake_option ENABLE_DBENGINE On
add_cmake_option ENABLE_ML On

add_cmake_option ENABLE_PLUGIN_APPS On
add_cmake_option ENABLE_PLUGIN_CGROUP_NETWORK On
add_cmake_option ENABLE_PLUGIN_DEBUGFS On
add_cmake_option ENABLE_PLUGIN_FREEIPMI On
add_cmake_option ENABLE_PLUGIN_GO On
add_cmake_option ENABLE_PLUGIN_SCRIPTS On
add_cmake_option ENABLE_PLUGIN_PYTHON On
add_cmake_option ENABLE_PLUGIN_CHARTS On
add_cmake_option ENABLE_PLUGIN_LOCAL_LISTENERS On
add_cmake_option ENABLE_PLUGIN_NFACCT On
add_cmake_option ENABLE_PLUGIN_NETFLOW On
add_cmake_option ENABLE_PLUGIN_OTEL On
add_cmake_option ENABLE_PLUGIN_PERF On
add_cmake_option ENABLE_PLUGIN_SLABINFO On
add_cmake_option ENABLE_PLUGIN_SYSTEMD_JOURNAL On
add_cmake_option ENABLE_PLUGIN_SYSTEMD_UNITS On

add_cmake_option ENABLE_EXPORTER_PROMETHEUS_REMOTE_WRITE On

add_cmake_option ENABLE_BUNDLED_PROTOBUF Off
add_cmake_option ENABLE_BUNDLED_JSONC Off
add_cmake_option ENABLE_BUNDLED_YAML Off

add_cmake_option ENABLE_LIBBACKTRACE On

add_cmake_option BUILD_FOR_PACKAGING On

[ -d "${SOURCE_DIR}/tmp/ibm_mq" ] && add_cmake_option FETCHCONTENT_SOURCE_DIR_IBM_MQ "${SOURCE_DIR}/tmp/ibm_mq"
if [ -n "${NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR}" ]; then
    if [ ! -d "${NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR}" ]; then
        echo "Missing topology IP intelligence stock directory: ${NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR}"
        exit 1
    fi
    add_cmake_option NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR "${NETDATA_TOPOLOGY_IP_INTEL_STOCK_DIR}"
fi

case "${PKG_TYPE}" in
    DEB)
        add_cmake_option NETDATA_PACKAGING_FORMAT deb

        case "$(dpkg-architecture -q DEB_TARGET_ARCH)" in
            amd64)
                add_cmake_option ENABLE_PLUGIN_XENSTAT On
                add_cmake_option ENABLE_PLUGIN_EBPF On
                add_cmake_option ENABLE_PLUGIN_IBM On
                ;;
            arm64)
                add_cmake_option ENABLE_PLUGIN_XENSTAT On
                add_cmake_option ENABLE_PLUGIN_EBPF Off
                add_cmake_option ENABLE_PLUGIN_IBM Off
                ;;
            armhf)
                add_cmake_option ENABLE_PLUGIN_XENSTAT Off
                add_cmake_option ENABLE_PLUGIN_EBPF Off
                add_cmake_option ENABLE_PLUGIN_IBM Off
                ;;
            *)
                add_cmake_option ENABLE_PLUGIN_XENSTAT Off
                add_cmake_option ENABLE_PLUGIN_EBPF Off
                add_cmake_option ENABLE_PLUGIN_IBM Off
                ;;
        esac

        if [ "${ID}" = "ubuntu" ]; then
            case "${VERSION_ID}" in
                20.04|22.04|24.04) add_cmake_option ENABLE_EXPORTER_MONGODB Off ;;
                *) add_cmake_option ENABLE_EXPORTER_MONGODB On ;;
            esac
        else
            add_cmake_option ENABLE_EXPORTER_MONGODB On
        fi
        ;;
    RPM)
        # Mirrors the per-distro conditionals of netdata.spec.in's %build.
        add_cmake_option NETDATA_PACKAGING_FORMAT rpm

        arch="$(uname -m)"
        distro_major="${VERSION_ID%%.*}"
        [ -n "${distro_major}" ] || distro_major=0

        is_el=0
        is_suse=0
        case "${ID}" in
            rhel|centos|almalinux|rocky|ol) is_el=1 ;;
            *suse*|sles) is_suse=1 ;;
        esac

        # "Legacy" here means EL <= 7 or Amazon Linux 2, the targets the
        # spec treats as the pre-weak-deps generation. Deliberate deviations
        # from the spec: EL 6 and i386 are not distinguished (both are dead
        # targets the v2 builder matrix does not contain).
        is_legacy_rpm=0
        if { [ "${is_el}" = 1 ] && [ "${distro_major}" -le 7 ]; } || \
           { [ "${ID}" = "amzn" ] && [ "${distro_major}" -le 2 ]; }; then
            is_legacy_rpm=1
        fi

        if [ "${arch}" = "x86_64" ]; then
            add_cmake_option ENABLE_PLUGIN_EBPF On
        else
            add_cmake_option ENABLE_PLUGIN_EBPF Off
        fi

        if [ "${arch}" = "x86_64" ] && [ "${is_legacy_rpm}" = 0 ]; then
            add_cmake_option ENABLE_PLUGIN_IBM On
        else
            add_cmake_option ENABLE_PLUGIN_IBM Off
        fi

        add_cmake_option ENABLE_PLUGIN_XENSTAT Off

        if [ "${is_legacy_rpm}" = 1 ]; then
            add_cmake_option ENABLE_PLUGIN_CUPS Off
            add_cmake_option ENABLE_PLUGIN_SYSTEMD_UNITS Off
        else
            add_cmake_option ENABLE_PLUGIN_CUPS On
            add_cmake_option ENABLE_PLUGIN_SYSTEMD_UNITS On
        fi

        if [ "${ID}" = "amzn" ] && [ "${distro_major}" -ge 2023 ]; then
            add_cmake_option ENABLE_PLUGIN_FREEIPMI Off
        else
            add_cmake_option ENABLE_PLUGIN_FREEIPMI On
        fi

        # nfacct ships only on Fedora and openSUSE Leap 15.x.
        if [ "${ID}" = "fedora" ] || { [ "${is_suse}" = 1 ] && [ "${distro_major}" -eq 15 ]; }; then
            add_cmake_option ENABLE_PLUGIN_NFACCT On
        else
            add_cmake_option ENABLE_PLUGIN_NFACCT Off
        fi

        if [ "${ID}" = "ol" ] || [ "${is_suse}" = 1 ] || [ "${ID}" = "amzn" ] || \
           { [ "${is_el}" = 1 ] && [ "${distro_major}" -ge 10 ]; }; then
            add_cmake_option ENABLE_EXPORTER_MONGODB Off
        else
            add_cmake_option ENABLE_EXPORTER_MONGODB On
        fi

        if [ "${is_suse}" = 1 ] || { [ "${is_el}" = 1 ] && [ "${distro_major}" -le 7 ]; }; then
            add_cmake_option ENABLE_BUNDLED_PROTOBUF On
        fi

        # RPM distros ship no static libprotobuf, and netdata's protobuf
        # detection prefers static libs; point it at the shared library
        # explicitly, exactly like the spec's %build does.
        for _pb in /usr/lib64/libprotobuf.so /usr/lib/libprotobuf.so; do
            if [ -e "${_pb}" ]; then
                add_cmake_option Protobuf_LIBRARY "${_pb}"
                break
            fi
        done

        if [ "${is_suse}" = 1 ]; then
            add_cmake_option USE_LTO Off
        fi

        if [ "${is_el}" = 1 ] && [ "${distro_major}" -le 7 ]; then
            add_cmake_option USE_CXX_11 On
            add_cmake_option FORCE_LEGACY_LIBBPF On
        fi

        # The spec builds through the distro %cmake macro. Reproduce its
        # build environment, or the binaries differ (missing hardening and
        # fortified symbols, extra sonames without --as-needed):
        # - distro compiler/linker flags from the rpm build macros;
        # - an empty CMAKE_BUILD_TYPE on the redhat macro family (it sets
        #   none), RelWithDebInfo on SUSE (its macro sets it).
        CFLAGS="${CFLAGS:-$(rpm -E '%{?build_cflags}')}"
        [ -n "${CFLAGS}" ] || CFLAGS="$(rpm -E '%{?optflags}')"
        CXXFLAGS="${CXXFLAGS:-$(rpm -E '%{?build_cxxflags}')}"
        [ -n "${CXXFLAGS}" ] || CXXFLAGS="${CFLAGS}"
        LDFLAGS="${LDFLAGS:-$(rpm -E '%{?build_ldflags}')}"
        export CFLAGS CXXFLAGS LDFLAGS

        # The distro %cmake macros also pass BUILD_SHARED_LIBS=ON, but the
        # top-level CMakeLists.txt unconditionally overwrites that variable
        # from STATIC_BUILD, so passing it here would be inert (it is equally
        # inert for the spec path, so parity holds without it).

        if [ "${is_suse}" = 0 ]; then
            add_cmake_option CMAKE_BUILD_TYPE ""
        else
            # SUSE's %cmake passes the linker flags as cmake arguments
            # rather than via LDFLAGS (its build_ldflags macro is empty).
            add_cmake_option CMAKE_EXE_LINKER_FLAGS " -Wl,--as-needed -Wl,-z,now"
            add_cmake_option CMAKE_MODULE_LINKER_FLAGS " -Wl,--as-needed"
            add_cmake_option CMAKE_SHARED_LINKER_FLAGS " -Wl,--as-needed -Wl,-z,now"
        fi
        ;;
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

run_cmake
cmake --build "${BUILD_DIR}" --parallel "$(nproc)" -- -k 1

if [ "${ENABLE_SENTRY}" = "true" ] && [ "${UPLOAD_SENTRY}" = "true" ]; then
    sentry-cli debug-files upload -o netdata-inc -p netdata-agent --force-foreground --log-level=debug --wait --include-sources build/netdata
fi

cd "${BUILD_DIR}" || exit 1
cpack -V -G "${PKG_TYPE}"
