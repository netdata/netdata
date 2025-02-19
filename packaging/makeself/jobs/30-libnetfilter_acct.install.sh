#!/usr/bin/env bash
# SPDX-License-Identifier: GPL-3.0-or-later
#
# Install the libnetfilter_acct

# shellcheck source=packaging/makeself/functions.sh
. "$(dirname "${0}")/../functions.sh" "${@}" || exit 1
# Source of truth for all the packages we bundle in static builds
. "$(dirname "${0}")/../bundled-packages.version" || exit 1

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::group::building libnetfilter_acct" || true

export CFLAGS="${TUNING_FLAGS} -static -pipe"
export CXXFLAGS="${CFLAGS}"
export LDFLAGS="-static"
export PKG_CONFIG="pkg-config --static"
export PKG_CONFIG_PATH="/libmnl-static/lib/pkgconfig"

cache_key="${LIBNETFILTER_ACCT_VERSION}"
fetch_git libnetfilter_acct "${LIBNETFILTER_ACCT_REPO}" "${LIBNETFILTER_ACCT_VERSION}" "${cache_key}"

if [ "${CACHE_HIT:-0}" -eq 0 ]; then
    run autoreconf -ivf

    run ./configure \
        --prefix="/libnetfilter-acct-static" \
        --exec-prefix="/libnetfilter-acct-static"

    run make clean
    run make -j "$(nproc)"
fi

run make install

store_cache "${cache_key}" "${NETDATA_MAKESELF_PATH}/tmp/libnetfilter_acct"

# shellcheck disable=SC2015
[ "${GITHUB_ACTIONS}" = "true" ] && echo "::endgroup::" || true
