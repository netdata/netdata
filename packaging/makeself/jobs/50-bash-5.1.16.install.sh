#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

version="5.1.16"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::building bash" || true

fetch "bash-${version}" "http://ftp.gnu.org/gnu/bash/bash-${version}.tar.gz" \
    5bac17218d3911834520dad13cd1f85ab944e1c09ae1aba55906be1f8192f558 bash

export CFLAGS="-pipe"
export PKG_CONFIG_PATH="/openssl-static/lib/pkgconfig"

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
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

    cat > examples/loadables/Makefile <<-EOF
	all:
	clean:
	install:
	EOF
fi

run make install

store_cache bash "${NETDATA_MAKESELF_PATH}/tmp/bash-${version}"

if [ "${NETDATA_BUILD_WITH_DEBUG}" -eq 0 ]; then
  run strip "${NETDATA_INSTALL_PATH}"/bin/bash
fi

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
