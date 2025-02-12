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

# Set correct CMake flags for building against non-System OpenSSL
# See: https://github.com/warmcat/libwebsockets/blob/master/READMEs/README.build.md
export CMAKE_FLAGS="-DOPENSSL_ROOT_DIR=/openssl-static -DOPENSSL_LIBRARIES=/openssl-static/lib64 -DCMAKE_INCLUDE_DIRECTORIES_PROJECT_BEFORE=/openssl-static -DLWS_OPENSSL_INCLUDE_DIRS=/openssl-static/include -DLWS_OPENSSL_LIBRARIES=/openssl-static/lib64/libssl.a;/openssl-static/lib64/libcrypto.a"

run ./netdata-installer.sh \
  --install-prefix "${NETDATA_INSTALL_PARENT}" \
  --dont-wait \
  --dont-start-it \
  --disable-exporting-mongodb \
  --use-system-protobuf \
  --dont-scrub-cflags-even-though-it-may-break-things \
  --one-time-build \
  --enable-lto \
  ${EXTRA_INSTALL_FLAGS:+${EXTRA_INSTALL_FLAGS}} \

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::Finishing netdata install" || true

# Properly mark the install type
cat > "${NETDATA_INSTALL_PATH}/etc/netdata/.install-type" <<-EOF
	INSTALL_TYPE='manual-static'
	PREBUILT_ARCH='${BUILDARCH}'
	EOF

# Remove the netdata.conf file from the tree. It has hard-coded sensible defaults builtin.
run rm -f "${NETDATA_INSTALL_PATH}/etc/netdata/netdata.conf"

# Ensure the netdata binary is in fact statically linked
if run readelf -l "${NETDATA_INSTALL_PATH}"/bin/netdata | grep 'INTERP'; then
  printf >&2 "Ooops. %s is not a statically linked binary!\n" "${NETDATA_INSTALL_PATH}"/bin/netdata
  ldd "${NETDATA_INSTALL_PATH}"/bin/netdata
  exit 1
fi

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
