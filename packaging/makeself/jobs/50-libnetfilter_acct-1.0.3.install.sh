#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Install the libnetfilter_acct and it's dependency libmnl
#

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1

version="1.0.3"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::building libnetfilter_acct" || true

export CFLAGS="-static -I/usr/include/libmnl -pipe"
export LDFLAGS="-static -L/usr/lib -lmnl"
export PKG_CONFIG="pkg-config --static"
export PKG_CONFIG_PATH="/usr/lib/pkgconfig"

fetch "libnetfilter_acct-${version}" "https://www.netfilter.org/projects/libnetfilter_acct/files/libnetfilter_acct-${version}.tar.bz2" \
    4250ceef3efe2034f4ac05906c3ee427db31b9b0a2df41b2744f4bf79a959a1a libnetfilter_acct


if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run ./configure \
        --prefix="/libnetfilter-acct-static" \
        --exec-prefix="/libnetfilter-acct-static"

    run make clean
    run make -j "$(nproc)"
fi

run make install

store_cache libnetfilter_acct "${NETDATA_MAKESELF_PATH}/tmp/libnetfilter_acct-${version}"


# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
