#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

cd "${NETDATA_SOURCE_PATH}" || exit 1

if [ "${NETDATA_BUILD_WITH_DEBUG}" -eq 0 ]; then
  export CFLAGS="${TUNING_FLAGS} -ffunction-sections -fdata-sections -static -O2 -funroll-loops -DNETDATA_STATIC_BUILD=1 -I/openssl-static/include -I/libnetfilter-acct-static/include/libnetfilter_acct -I/curl-local/include/curl -I/usr/include/libmnl -pipe"
else
  export CFLAGS="${TUNING_FLAGS} -static -O1 -pipe -ggdb -Wall -Wextra -Wformat-signedness -DNETDATA_STATIC_BUILD=1 -DNETDATA_INTERNAL_CHECKS=1 -I/openssl-static/include -I/libnetfilter-acct-static/include/libnetfilter_acct -I/curl-local/include/curl -I/usr/include/libmnl"
fi

export LDFLAGS="-Wl,--gc-sections -static -L/openssl-static/lib64 -L/libnetfilter-acct-static/lib -lnetfilter_acct -L/usr/lib -lmnl -L/usr/lib -lzstd -L/curl-local/lib"

# We export this to 'yes', installer sets this to .environment.
# The updater consumes this one, so that it can tell whether it should update a static install or a non-static one
export IS_NETDATA_STATIC_BINARY="yes"

# Set eBPF LIBC to "static" to bundle the `-static` variant of the kernel-collector
export EBPF_LIBC="static"
export PKG_CONFIG="pkg-config --static"
export PKG_CONFIG_PATH="/openssl-static/lib64/pkgconfig:/libnetfilter-acct-static/lib/pkgconfig:/usr/lib/pkgconfig:/curl-local/lib/pkgconfig"

NETDATA_BUILD_DIR="$(build_path netdata)"
export NETDATA_BUILD_DIR

case "${BUILDARCH}" in
    armv6l)
        export NETDATA_CMAKE_OPTIONS="-DENABLE_LIBBACKTRACE=On"
        export INSTALLER_ARGS="--disable-plugin-systemd-journal"
        ;;
    *)
        export NETDATA_CMAKE_OPTIONS="-DENABLE_LIBBACKTRACE=On"
        export INSTALLER_ARGS="--enable-plugin-systemd-journal --internal-systemd-journal"
        ;;
esac

export RUSTFLAGS="-C target-feature=+crt-static"

run ./netdata-installer.sh \
  --install-prefix "${NETDATA_INSTALL_PARENT}" \
  --dont-wait \
  --dont-start-it \
  --disable-exporting-mongodb \
  --enable-plugin-systemd-journal \
  --dont-scrub-cflags-even-though-it-may-break-things \
  --one-time-build \
  --enable-lto \
  ${INSTALLER_ARGS:+${INSTALLER_ARGS}} \
  ${EXTRA_INSTALL_FLAGS:+${EXTRA_INSTALL_FLAGS}} \
