#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=./packaging/makeself/functions.sh
. "${NETDATA_MAKESELF_PATH}"/functions.sh "${@}" || exit 1

cd "${NETDATA_SOURCE_PATH}" || exit 1

COMMON_CFLAGS="-static -I/libunwind-static/include -I/openssl-static/include -I/libmnl-static/include -I/libnetfilter-acct-static/include/libnetfilter_acct -I/curl-local/include/curl -pipe"

if [ "${NETDATA_BUILD_WITH_DEBUG}" -eq 0 ]; then
  export CFLAGS="${TUNING_FLAGS} ${COMMON_CFLAGS} -ffunction-sections -fdata-sections -O2 -funroll-loops"
else
  export CFLAGS="${TUNING_FLAGS} ${COMMON_CFLAGS} -O1 -ggdb -Wall -Wextra -Wformat-signedness -DNETDATA_INTERNAL_CHECKS=1"
fi

export LDFLAGS="-Wl,--gc-sections -static -L/libuwnind-static/lib -L/openssl-static/lib64 -L/libmnl-static/lib -L/libnetfilter-acct-static/lib -L/curl-local/lib"

# We export this to 'yes', installer sets this to .environment.
# The updater consumes this one, so that it can tell whether it should update a static install or a non-static one
export IS_NETDATA_STATIC_BINARY="yes"

# Set eBPF LIBC to "static" to bundle the `-static` variant of the kernel-collector
export EBPF_LIBC="static"
export PKG_CONFIG="pkg-config --static"
export PKG_CONFIG_PATH="/libunwind-static/lib/pkgconfig:/openssl-static/lib64/pkgconfig:/libmnl-static/lib/pkgconfig:/libnetfilter-acct-static/lib/pkgconfig:/usr/lib/pkgconfig:/curl-local/lib/pkgconfig"

[ "${BUILDARCH}" != "ppc64le" ] && export NETDATA_CMAKE_OPTIONS="-DENABLE_LIBUNWIND=ON"

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
