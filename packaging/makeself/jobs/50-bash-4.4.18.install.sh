#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

fetch "bash-4.4.18" "http://ftp.gnu.org/gnu/bash/bash-4.4.18.tar.gz"

export PKG_CONFIG_PATH="/openssl-static/lib/pkgconfig"

run ./configure \
  --prefix="${NETDATA_INSTALL_PATH}" \
  --without-bash-malloc \
  --enable-static-link \
  --enable-net-redirections \
  --enable-array-variables \
  --disable-profiling \
  --disable-nls

run make clean
run make -j "$(nproc)"

cat > examples/loadables/Makefile << EOF
all:
clean:
install:
EOF

run make install

if [ ${NETDATA_BUILD_WITH_DEBUG} -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/bin/bash
fi
