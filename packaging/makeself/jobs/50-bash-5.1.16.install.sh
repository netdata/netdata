#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::building bash" || true

fetch "bash-5.1.16" "http://ftp.gnu.org/gnu/bash/bash-5.1.16.tar.gz" \
    5bac17218d3911834520dad13cd1f85ab944e1c09ae1aba55906be1f8192f558

export CFLAGS="-pipe"
export PKG_CONFIG_PATH="/openssl-static/lib/pkgconfig"

run ./configure \
  --prefix="${NETDATA_INSTALL_PATH}" \
  --without-bash-malloc \
  --enable-static-link \
  --enable-net-redirections \
  --enable-array-variables \
  --disable-progcomp \
  --disable-profiling \
  --disable-nls \
  --disable-dependency-tracking

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
