#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

fetch "fping-5.0" "https://fping.org/dist/fping-5.0.tar.gz"

export CFLAGS="-static -I/openssl-static/include"
export LDFLAGS="-static -L/openssl-static/lib"
export PKG_CONFIG_PATH="/openssl-static/lib/pkgconfig"

run ./configure \
  --prefix="${NETDATA_INSTALL_PATH}" \
  --enable-ipv4 \
  --enable-ipv6

cat > doc/Makefile << EOF
all:
clean:
install:
EOF

run make clean
run make -j "$(nproc)"
run make install

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/bin/fping
fi
