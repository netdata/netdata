#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

cd "${NETDATA_SOURCE_PATH}" || exit 1

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]; then
  export CFLAGS="-static -O3 -I/openssl-static/include"
else
  export CFLAGS="-static -O1 -ggdb -Wall -Wextra -Wformat-signedness -fstack-protector-all -D_FORTIFY_SOURCE=2 -DNETDATA_INTERNAL_CHECKS=1 -I/openssl-static/include"
fi

export LDFLAGS="-static -L/openssl-static/lib"

# We export this to 'yes', installer sets this to .environment.
# The updater consumes this one, so that it can tell whether it should update a static install or a non-static one
export IS_NETDATA_STATIC_BINARY="yes"

# Set eBPF LIBC to "static" to bundle the `-static` variant of the kernel-collector
export EBPF_LIBC="static"
export PKG_CONFIG_PATH="/openssl-static/lib/pkgconfig"

# Set correct CMake flags for building against non-System OpenSSL
# See: https://github.com/warmcat/libwebsockets/blob/master/READMEs/README.build.md
export CMAKE_FLAGS="-DOPENSSL_ROOT_DIR=/openssl-static -DOPENSSL_LIBRARIES=/openssl-static/lib -DCMAKE_INCLUDE_DIRECTORIES_PROJECT_BEFORE=/openssl-static -DLWS_OPENSSL_INCLUDE_DIRS=/openssl-static/include -DLWS_OPENSSL_LIBRARIES=/openssl-static/lib/libssl.a;/openssl-static/lib/libcrypto.a"

run ./netdata-installer.sh \
  --install "${NETDATA_INSTALL_PARENT}" \
  --dont-wait \
  --dont-start-it \
  --require-cloud \
  --dont-scrub-cflags-even-though-it-may-break-things

# Remove the netdata.conf file from the tree. It has hard-coded sensible defaults builtin.
run rm -f "${NETDATA_INSTALL_PATH}/etc/netdata/netdata.conf"

# Ensure the netdata binary is in fact statically linked
if run readelf -l "${NETDATA_INSTALL_PATH}"/bin/netdata | grep 'INTERP'; then
  printf >&2 "Ooops. %s is not a statically linked binary!\n" "${NETDATA_INSTALL_PATH}"/bin/netdata
  ldd "${NETDATA_INSTALL_PATH}"/bin/netdata
  exit 1
fi

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/bin/netdata
  run strip "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/apps.plugin
  run strip "${NETDATA_INSTALL_PATH}"/usr/libexec/netdata/plugins.d/cgroup-network
fi
