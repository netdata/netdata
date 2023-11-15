#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Install the libnetfilter_acct and it's dependency libmnl


# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1
# Source of truth for all the packages we bundle in static builds
. "$(dirname "${0}")/../bundled-packages" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::building libnetfilter_acct" || true

export CFLAGS="-static -I/usr/include/libmnl -pipe"
export LDFLAGS="-static -L/usr/lib -lmnl"
export PKG_CONFIG="pkg-config --static"
export PKG_CONFIG_PATH="/usr/lib/pkgconfig"

fetch "libnetfilter_acct-${LIBNETFILTER_ACT_VERSION}" "${LIBNETFILTER_ACT_SOURCE}/libnetfilter_acct-${LIBNETFILTER_ACT_VERSION}.tar.bz2" \
    "${LIBNETFILTER_ACT_ARTIFACT_SHA256}" libnetfilter_acct


if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run ./configure \
        --prefix="/libnetfilter-acct-static" \
        --exec-prefix="/libnetfilter-acct-static"

    run make clean
    run make -j "$(nproc)"
fi

run make install

store_cache libnetfilter_acct "${NETDATA_MAKESELF_PATH}/tmp/libnetfilter_acct-${LIBNETFILTER_ACT_VERSION}"


# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
