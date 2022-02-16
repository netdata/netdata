#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::building bash" || true

fetch "bash-5.1.8" "http://ftp.gnu.org/gnu/bash/bash-5.1.8.tar.gz" \
    0cfb5c9bb1a29f800a97bd242d19511c997a1013815b805e0fdd32214113d6be

export PKG_CONFIG_PATH="/openssl-static/lib/pkgconfig"

run ./configure \
  --prefix="${NETDATA_INSTALL_PATH}" \
  --without-bash-malloc \
  --enable-static-link \
  --enable-net-redirections \
  --enable-array-variables \
  --disable-progcomp \
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

if [ "${NETDATA_BUILD_WITH_DEBUG}" -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/bin/bash
fi

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
